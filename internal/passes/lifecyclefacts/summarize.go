package lifecyclefacts

import (
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Summary inference composes ownership, retention and exact cleanup evidence
// into a function's lifecycle fact. Cleanup paths retain their own coverage
// proof, and a callback or cutoff cannot become an unconditional discharge.

func (callbacks *callbackInference) summarize(function *ssa.Function) Fact {
	pass := callbacks.pass
	// The transfer and retention claims are read from the projection, so it
	// and the signature are attached before any proof below reads them.
	heap := projectHeap(function)
	fact := Fact{Heap: heap, signature: function.Signature}
	fact.Must.OwnedFields = ownedFields(pass, function)
	fact.Must.ReleasedFields = releasedFields(pass, function)
	fact.Must.OwnedResults = ownedResults(pass, function)
	fact.Must.RetainingResults = retainingResults(pass, function)
	invocation := callbacks.invocations.Function(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	fact.Discharges = append(fact.Discharges, invocation.Discharges...)
	// A fact is exported only when the action is unavoidable on every normal
	// return. Each mask is therefore proved independently; evidence for Close,
	// for example, must never make an unrelated Wait or return-transfer claim true.
	for index, parameter := range function.Params {
		if !ownershipCapableType(parameter.Type()) {
			continue
		}
		bit := parameterMaskFor(index)
		summarizeDischarges(pass, function, index, parameter, &fact)
		if releasesDerivedValueInLoop(function, parameter) {
			fact.May.LoopReleased |= bit
		}
	}
	fact.Discharges = append(fact.Discharges, withoutUnconditional(summarizeConditional(pass, function), fact.Discharges)...)
	fact.ReturnedCleanup = summarizeReturnedCleanup(pass, function)
	fact.Heap = withReleases(heap, &fact)
	return fact
}

// summarizeDischarges records, for each lifecycle method, the paths beneath
// the parameter it cleans up on every return, and the empty path when it
// cleans up the parameter itself.
func summarizeDischarges(pass *analysis.Pass, function *ssa.Function, index int, parameter ssa.Value, fact *Fact) {
	for _, method := range cleanupMethods {
		if releasesEachElement(function, parameter, method) {
			fact.Discharges = append(fact.Discharges, Discharge{Parameter: index, Method: method, Path: EachElementPath})
		}
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
			// A nested helper whose case the call's constant arguments select
			// settles the parameter as surely as one that always does.
			return ok && (imported.dischargesArgument(instruction, parameter, method, nil) ||
				imported.caseDischargesArgument(instruction, parameter, method, nil, nil))
		}) {
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
		proof := lifecycle.ProveCompletion(lifecycle.CompletionRequest{
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
			path, ok := heapmodel.AccessPathFromParameter(ssaflow.CallReceiver(common), parameter)
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
	actual, ok := heapmodel.AccessPathFromParameter(ssaflow.CallReceiver(common), parameter)
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
		if !ok || !strings.HasPrefix(function.Synthetic, "bound method wrapper for ") || !lifecycle.ValueCallsMethod(closure, method, target) {
			continue
		}
		if lifecycle.ProveCompletion(lifecycle.CompletionRequest{
			Instruction: instruction, Target: closure, InvokeTarget: true, Budget: ssaflow.NewSearchBudget(ssaflow.QueryBudget),
		}).Proven() {
			return true
		}
	}
	return false
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
	return lifecycle.MethodCallCoverage(function, owns, lifecycle.CoverageEveryReturn, parameter)
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
	return ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{
		Entry: function,
		Owns:  func(ssa.Instruction) bool { return false },
		AllowReturn: func(returned *ssa.Return) bool {
			return lifecycle.ReturnedValueOwnsValueSummarized(returned, parameter, summarized) || ssaflow.ReturnsOnlyNilOrErrors(returned)
		},
	}) == nil
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
