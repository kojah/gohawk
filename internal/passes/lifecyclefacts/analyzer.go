// Package lifecyclefacts resolves lifecycle evidence across source and package
// boundaries. It combines memoized local SSA proofs with conservative exported
// summaries, and keeps missing summaries distinct from disproved ownership.
package lifecyclefacts

import (
	"go/types"
	"reflect"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	ssa "golang.org/x/tools/go/ssa"
)

// Analyzer is an internal prerequisite shared by lifecycle analyzers.
var Analyzer = &analysis.Analyzer{
	Name:       "gohawklifecyclefacts",
	Doc:        "exports internal lifecycle ownership summaries",
	Requires:   []*analysis.Analyzer{buildssa.Analyzer},
	FactTypes:  []analysis.Fact{new(Fact), new(CleanupFact), new(SummarizedPackage)},
	ResultType: reflect.TypeFor[Summaries](),
	Run:        run,
}

// traceAnalyzer names this prerequisite in a trace. It is not a catalog
// analyzer, but a reader selects its events the same way: -gohawk-trace=
// lifecyclefacts shows which function the fact pass is working on.
const traceAnalyzer = "lifecyclefacts"

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	summaries := make(Summaries, len(functions))
	retentions := newRetentionCache()
	marker := &SummarizedPackage{}
	// Import dependency summaries first, and hand their heap projections to
	// the graph, so every graph built for this package applies a summarized
	// callee by substitution instead of forgetting what the caller holds.
	// Facts belong to this prerequisite analyzer; siblings read them through
	// the result. A callee of this package has no fact yet and stays
	// unknown here; its own summary is registered once computed.
	importCalleeSummaries(pass, functions, summaries)
	var local []*ssa.Function
	for _, function := range functions {
		if object := function.Object(); object != nil && object.Exported() && len(function.Blocks) == 0 {
			marker.Bodiless = append(marker.Bodiless, object.Name())
		}
		object := function.Object()
		// Only exported functions can be called from a package that imports this
		// fact. Skipping private dependency helpers keeps the prerequisite linear
		// in the externally visible API instead of every transitive SSA body.
		if object == nil || !object.Exported() || len(function.Params) > 64 || len(function.Blocks) == 0 {
			continue
		}
		// Summarizing walks the callee graph, so a pathological package can
		// spend a long time on one function. Announce the function before the
		// walk as well as after it: a run that stops making progress is then
		// located by its last candidate rather than by a stack dump.
		probe := analysisTrace.For(pass, traceAnalyzer, "", function.Pos())
		probe.Candidate(analysisTrace.Step{
			Reason:   "summarizing-function",
			Outcome:  analysisTrace.OutcomeObserved,
			Pos:      function.Pos(),
			Function: function.String(),
		})
		fact := summarize(pass, retentions, function)
		summaries[function] = fact
		local = append(local, function)
		probe.Decision(analysisTrace.Step{
			Reason:   "function-summarized",
			Outcome:  analysisTrace.OutcomeAccepted,
			Pos:      function.Pos(),
			Function: function.String(),
			Details:  fact.traceDetails(),
		})
	}
	// A returned view is decided once every method of this package is
	// summarized, because the releasing method usually lives beside the
	// constructor. Export afterwards. An importer must be able to tell a
	// callee proven to do nothing from one that was never summarized, and
	// only the latter is unknown; the package marker carries that
	// distinction, so an empty summary need not be serialized for every
	// function of every dependency.
	slices.Sort(marker.Bodiless)
	pass.ExportPackageFact(marker)
	for _, function := range local {
		fact := summaries[function]
		fact.ReturnedView = returnedViews(pass, function, fact, summaries)
		summaries[function] = fact
		if !fact.empty() {
			pass.ExportObjectFact(function.Object(), &fact)
		}
		if fact.Heap != nil {
			ssaflow.RegisterHeapSummary(function, *fact.Heap)
		}
	}
	// A type's contract needs its constructor and its methods, so it is joined
	// once both are summarized rather than while either is being proved.
	exportCleanupContracts(pass, summaries)
	return summaries, nil
}

// importCalleeSummaries imports the fact of every static callee the package
// resolves and registers each callee's heap projection with the graph.
func importCalleeSummaries(pass *analysis.Pass, functions []*ssa.Function, summaries Summaries) {
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				common := ssaflow.InstructionCall(instruction)
				if common == nil || common.StaticCallee() == nil {
					continue
				}
				if _, seen := summaries[common.StaticCallee()]; seen {
					continue
				}
				if fact, ok := importFact(pass, instruction); ok {
					summaries[common.StaticCallee()] = fact
					if fact.Heap != nil {
						ssaflow.RegisterHeapSummary(common.StaticCallee(), *fact.Heap)
					}
					// A callee that returns an owned struct is only useful together
					// with the summaries of that struct's methods, which no sibling
					// analyzer can import itself.
					if fact.OwnedFields != 0 {
						importResultMethods(pass, common.StaticCallee(), summaries)
					}
				}
			}
		}
	}
}

// factFor returns a memoized local or previously imported dependency summary.
func factFor(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	common := ssaflow.InstructionCall(instruction)
	if pass == nil || common == nil || common.StaticCallee() == nil {
		return Fact{}, false
	}
	if summaries, ok := pass.ResultOf[Analyzer].(Summaries); ok {
		if fact, found := summaries[common.StaticCallee()]; found {
			return fact, true
		}
	}
	return Fact{}, false
}

func summarize(pass *analysis.Pass, retentions *retentionCache, function *ssa.Function) Fact {
	var fact Fact
	heap := projectHeap(function)
	fact.OwnedFields = ownedFields(pass, function)
	fact.ReleasedFields = releasedFields(pass, function)
	fact.OwnedResults = ownedResults(pass, function)
	// A fact is exported only when the action is unavoidable on every normal
	// return. Each mask is therefore proved independently; evidence for Close,
	// for example, must never make an unrelated Wait or return-transfer claim true.
	for index, parameter := range function.Params {
		if !ownershipCapableType(parameter.Type()) {
			continue
		}
		bit := parameterMaskFor(index)
		invokes := func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			if common != nil && ssaflow.NewStorage(nil).Same(common.Value, parameter).Proven() {
				return true
			}
			imported, ok := importFact(pass, instruction)
			return ok && factOwnsExactArgument(instruction, parameter, imported.Invoked)
		}
		if ownsOnEveryReturn(function, parameter, invokes) {
			fact.Invoked |= bit
		}
		if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
			return synchronouslyInvokesParameter(pass, instruction, parameter)
		}) {
			fact.SynchronouslyInvoked |= bit
		}
		summarizeDischarges(pass, function, index, parameter, &fact)
		if releasesDerivedValueInLoop(function, parameter) {
			fact.LoopReleased |= bit
		}
		summarizeTransfers(pass, retentions, heap, function, index, parameter, &fact)
	}
	fact.Conditional = summarizeConditional(pass, function)
	fact.ReturnedCleanup = summarizeReturnedCleanup(pass, function)
	fact.Heap = withReleases(heap, &fact)
	return fact
}

// summarizeDischarges records, for each lifecycle method, the paths beneath
// the parameter it cleans up on every return and, for the parameter itself,
// the method's mask.
func summarizeDischarges(pass *analysis.Pass, function *ssa.Function, index int, parameter ssa.Value, fact *Fact) {
	bit := parameterMaskFor(index)
	for _, mask := range lifecycleMasks {
		if mask.method == "" {
			continue
		}
		method, target := mask.method, mask.field(fact)
		deferred := deferredCompletions(function, parameter, method)
		// A cleanup of a field or element is claimed at its own path,
		// never as a cleanup of the parameter: closing j.out is not
		// closing j, and a caller whose file sits in j.other must not be
		// credited. Only the parameter itself sets the mask.
		for _, path := range cleanupPaths(function, parameter, method, deferred) {
			if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
				settled, ok := deferred[instruction]
				return ok && settled == path || cleanupAtPath(instruction, parameter, method, path)
			}) {
				fact.Discharges = append(fact.Discharges, Discharge{Parameter: index, Method: method, Path: path})
			}
		}
		if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
			if cleanupAtPath(instruction, parameter, method, "") {
				return true
			}
			if settled, ok := deferred[instruction]; ok && settled == "" {
				return true
			}
			if invokesMethodCallback(instruction, parameter, method) {
				return true
			}
			imported, ok := importFact(pass, instruction)
			return ok && imported.dischargesArgument(instruction, parameter, method, nil)
		}) {
			*target |= bit
			fact.Discharges = append(fact.Discharges, Discharge{Parameter: index, Method: method})
		}
	}
}

// deferredCompletions maps each deferred launch that the local completion
// proof shows calls method on the parameter to the path beneath the
// parameter it settles. This exports the same deferred-callback evidence
// the local lifecycle proofs accept. Qist's response helper defers a
// literal that closes the Body projected from its response parameter on
// every return:
// https://github.com/qist/tvgate/blob/bb4c889997c68cc607d9ab5bb34710d6baf94aa8/stream/handle.go#L31-L36
// Before the proof reported a path, that literal claimed the response
// itself; it now claims the body's path, and a caller is credited for the
// resource it stored there. A completion whose path the proof cannot name,
// because the receiver has no static path or different returns settle
// different fields, is not exported at all: a claim on the parameter would
// credit a caller for a resource the helper never touched. An exhausted
// budget likewise exports nothing.
func deferredCompletions(function *ssa.Function, parameter ssa.Value, method string) map[ssa.Instruction]string {
	completions := map[ssa.Instruction]string{}
	for _, instruction := range ssaflow.InstructionsOf[*ssa.Defer](function) {
		proof := ssaflow.ProveCompletion(ssaflow.CompletionRequest{
			Instruction: instruction, Target: parameter, Methods: []string{method},
			Budget: ssaflow.NewSearchBudget(ssaflow.SummaryBudget),
		})
		if proof.Proven() && proof.PathKnown {
			completions[instruction] = proof.Path
		}
	}
	return completions
}

// cleanupPaths returns the non-empty access paths beneath the parameter on
// which the function calls method, directly or through a deferred
// completion: the fields and constant-index elements it cleans up, each a
// candidate for its own every-return claim.
func cleanupPaths(function *ssa.Function, parameter ssa.Value, method string, deferred map[ssa.Instruction]string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, path := range deferred {
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if common == nil || ssaflow.CallName(common) != method {
				continue
			}
			path, ok := ssaflow.AccessPathFromParameter(ssaflow.CallReceiver(common), parameter)
			if !ok || len(path) == 0 {
				continue
			}
			if joined := ssaflow.JoinAccessPath(path); !seen[joined] {
				seen[joined] = true
				paths = append(paths, joined)
			}
		}
	}
	return paths
}

// cleanupAtPath reports whether the instruction calls method on the value
// at exactly path beneath the parameter; the empty path is the parameter
// itself.
func cleanupAtPath(instruction ssa.Instruction, parameter ssa.Value, method, path string) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || ssaflow.CallName(common) != method {
		return false
	}
	actual, ok := ssaflow.AccessPathFromParameter(ssaflow.CallReceiver(common), parameter)
	return ok && ssaflow.JoinAccessPath(actual) == path
}

// A visible helper may invoke a bound cleanup method supplied by its wrapper.
// Export that same completion proof rather than losing it at the package edge.
// A single exact capture excludes callbacks containing a possible owner alias.
// InvokeTarget requires this callback on every return, including when a helper
// selects among function values; merely passing the callback is not completion.
// https://github.com/opencontainers/umoci/blob/f5d1219acaf67127ebacf6306776d3ff465735ea/internal/funchelpers/verify_error.go#L55-L66
func invokesMethodCallback(instruction ssa.Instruction, target ssa.Value, method string) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	for _, argument := range call.Common().Args {
		closure, ok := argument.(*ssa.MakeClosure)
		if !ok || len(closure.Bindings) != 1 || closure.Bindings[0] != target {
			continue
		}
		// x/tools uses this synthetic wrapper for a method value. Restrict the
		// candidate to that small body instead of searching arbitrary literals
		// once per parameter and lifecycle method during summary construction.
		function, ok := closure.Fn.(*ssa.Function)
		if !ok || !strings.HasPrefix(function.Synthetic, "bound method wrapper for ") || !ssaflow.ValueCallsMethod(closure, method, target) {
			continue
		}
		if ssaflow.ProveCompletion(ssaflow.CompletionRequest{
			Instruction: instruction, Target: closure, InvokeTarget: true, Budget: ssaflow.NewSearchBudget(ssaflow.QueryBudget),
		}).Proven() {
			return true
		}
	}
	return false
}

func synchronouslyInvokesParameter(pass *analysis.Pass, instruction ssa.Instruction, parameter ssa.Value) bool {
	if _, asynchronous := instruction.(*ssa.Go); asynchronous {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	if common != nil && ssaflow.NewStorage(nil).Same(common.Value, parameter).Proven() {
		return true
	}
	imported, ok := importFact(pass, instruction)
	return ok && factOwnsExactArgument(instruction, parameter, imported.SynchronouslyInvoked)
}

// summarizeTransfers records where a parameter goes: into the returned
// owner, into the receiver, or kept somewhere by the callee.
func summarizeTransfers(
	pass *analysis.Pass,
	retentions *retentionCache,
	heap *ssaflow.HeapSummary,
	function *ssa.Function,
	index int,
	parameter ssa.Value,
	fact *Fact,
) {
	bit := parameterMaskFor(index)
	if returnedOwnerOnEveryReturn(pass, function, parameter) {
		fact.ReturnedOwner |= bit
	}
	if index > 0 && receiverStores(heap, index) {
		fact.ReceiverStore |= bit
	}
	if retentions.retainedAnywhere(pass, function, parameter) {
		fact.Retained |= bit
	} else if structShaped(parameter.Type()) {
		// Only an aggregate has contents, and a retained parameter already
		// keeps all of them.
		for _, path := range retentions.keptPaths(pass, function, parameter) {
			fact.Kept = append(fact.Kept, Kept{Parameter: index, Path: path})
		}
	}
	if retentions.storedAnywhere(pass, function, parameter) {
		fact.Stored |= bit
	}
}

// structShaped reports whether a parameter of this type is a struct or a
// pointer to one, the shapes whose contents a caller can hand over whole.
func structShaped(parameterType types.Type) bool {
	if pointer, ok := parameterType.Underlying().(*types.Pointer); ok {
		parameterType = pointer.Elem()
	}
	_, ok := parameterType.Underlying().(*types.Struct)
	return ok
}

func ownsOnEveryReturn(function *ssa.Function, parameter ssa.Value, owns func(ssa.Instruction) bool) bool {
	// The absence of an unowned return is vacuous for panic-only or infinite
	// bodies. Use the shared completion coverage, which also requires an action
	// witness, before advertising a lifecycle action to another package.
	return ssaflow.MethodCallCoverage(function, owns, ssaflow.CoverageEveryReturn, parameter)
}

func returnedOwnerOnEveryReturn(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	if !canReturnOwner(function.Signature.Results()) || len(function.Blocks) == 0 || !ssaflow.NormalReturnReachableFrom(function.Blocks[0]) {
		return false
	}
	// A constructor commonly delegates across a package boundary, so the
	// search needs the callee's summary where its body is unavailable.
	summarized := func(callee *ssa.Function, index int) bool {
		imported, ok := factForFunction(pass, callee)
		return ok && imported.Claim(ClaimReturnsOwner).contains(index)
	}
	return !ssaflow.UnownedReturnFromEntryAllow(function, func(ssa.Instruction) bool { return false }, func(returned *ssa.Return) bool {
		return ssaflow.ReturnedValueOwnsValueSummarized(returned, parameter, summarized) || allResultsNil(returned)
	})
}

func canReturnOwner(results *types.Tuple) bool {
	for result := range results.Variables() {
		if types.Identical(result.Type(), types.Universe.Lookup("error").Type()) {
			continue
		}
		if ownershipCapableType(result.Type()) {
			return true
		}
	}
	return false
}

func ownershipCapableType(value types.Type) bool {
	switch value.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Signature, *types.Map, *types.Slice, *types.Chan, *types.Struct, *types.Array:
		return true
	default:
		return false
	}
}

func allResultsNil(returned *ssa.Return) bool {
	if len(returned.Results) == 0 {
		return false
	}
	for _, result := range returned.Results {
		if !ssaflow.DefinitelyNil(result) && result.Type().String() != "error" {
			return false
		}
	}
	return true
}
