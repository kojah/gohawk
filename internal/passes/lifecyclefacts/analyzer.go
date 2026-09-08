// Package lifecyclefacts resolves lifecycle evidence across source and package
// boundaries. It combines memoized local SSA proofs with conservative exported
// summaries, and keeps missing summaries distinct from disproved ownership.
package lifecyclefacts

import (
	"go/types"
	"reflect"

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
	FactTypes:  []analysis.Fact{new(Fact), new(CleanupFact)},
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
	for _, function := range functions {
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
	// constructor. Export afterwards, and export even an empty summary: an
	// importer must be able to tell a callee proven to do nothing from one that
	// was never summarized, and only the latter is unknown.
	for function, fact := range summaries {
		fact.ReturnedView = returnedViews(pass, function, fact, summaries)
		summaries[function] = fact
		pass.ExportObjectFact(function.Object(), &fact)
	}
	// A type's contract needs its constructor and its methods, so it is joined
	// once both are summarized rather than while either is being proved.
	exportCleanupContracts(pass, summaries)
	// Facts belong to this prerequisite analyzer, so import dependency facts
	// here and expose them through the result consumed by sibling analyzers.
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				common := ssaflow.InstructionCall(instruction)
				if common == nil || common.StaticCallee() == nil {
					continue
				}
				if fact, ok := importFact(pass, instruction); ok {
					summaries[common.StaticCallee()] = fact
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
	return summaries, nil
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
	fact.OwnedFields = ownedFields(pass, function)
	fact.ReleasedFields = releasedFields(pass, function)
	// A fact is exported only when the action is unavoidable on every normal
	// return. Each mask is therefore proved independently; evidence for Close,
	// for example, must never make an unrelated Wait or return-transfer claim true.
	for index, parameter := range function.Params {
		if !ownershipCapableType(parameter.Type()) {
			continue
		}
		bit := parameterMaskFor(index)
		if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			if common != nil && ssaflow.SameValue(common.Value, parameter) {
				return true
			}
			imported, ok := importFact(pass, instruction)
			return ok && factOwnsArgument(instruction, parameter, imported.Invoked)
		}) {
			fact.Invoked |= bit
		}
		for _, mask := range lifecycleMasks {
			if mask.method == "" {
				continue
			}
			method, target := mask.method, mask.field(&fact)
			if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
				common := ssaflow.InstructionCall(instruction)
				if common != nil && ssaflow.CallName(common) == method &&
					ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), parameter, map[ssa.Value]bool{}) {
					return true
				}
				// Export the same exact deferred-callback evidence accepted by local
				// lifecycle proofs. Qist's response helper defers a literal that closes
				// the Body projected from its response parameter on every return:
				// https://github.com/qist/tvgate/blob/bb4c889997c68cc607d9ab5bb34710d6baf94aa8/stream/handle.go#L31-L36
				if _, deferred := instruction.(*ssa.Defer); deferred && ssaflow.ProveCompletion(ssaflow.CompletionRequest{
					Instruction: instruction, Target: parameter, Methods: []string{method},
				}).Proven() {
					return true
				}
				imported, ok := importFact(pass, instruction)
				return ok && factOwnsArgument(instruction, parameter, imported.MethodMask(method))
			}) {
				*target |= bit
			}
		}
		summarizeTransfers(pass, retentions, function, index, parameter, &fact)
	}
	return fact
}

// summarizeTransfers records where a parameter goes: into the returned
// owner, into the receiver, or kept somewhere by the callee.
func summarizeTransfers(
	pass *analysis.Pass,
	retentions *retentionCache,
	function *ssa.Function,
	index int,
	parameter ssa.Value,
	fact *Fact,
) {
	bit := parameterMaskFor(index)
	if returnedOwnerOnEveryReturn(pass, function, parameter) {
		fact.ReturnedOwner |= bit
	}
	if index > 0 && storedInReceiverOnEveryReturn(function, function.Params[0], parameter) {
		fact.ReceiverStore |= bit
	}
	if retentions.retainedAnywhere(pass, function, parameter) {
		fact.Retained |= bit
	}
	if retentions.storedAnywhere(pass, function, parameter) {
		fact.Stored |= bit
	}
}

func ownsOnEveryReturn(function *ssa.Function, parameter ssa.Value, owns func(ssa.Instruction) bool) bool {
	return !ssaflow.UnownedReturnFromEntryAssumingNonNil(function, parameter, owns)
}

func returnedOwnerOnEveryReturn(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	if !canReturnOwner(function.Signature.Results()) {
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

func storedInReceiverOnEveryReturn(function *ssa.Function, receiver, parameter ssa.Value) bool {
	return !ssaflow.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
		store, ok := instruction.(*ssa.Store)
		if !ok || !ssaflow.ValueDerivesFrom(store.Val, parameter, map[ssa.Value]bool{}) {
			return false
		}
		field, ok := store.Addr.(*ssa.FieldAddr)
		return ok && ssaflow.ValueDerivesFrom(field.X, receiver, map[ssa.Value]bool{})
	})
}
