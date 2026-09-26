package lifecycle

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// CompletionSummaryLookup supplies positive guarantees for unavailable callees.
// The implementation must bind target to an exact argument, distinguish the
// requested method from callback invocation, and match the query condition:
// the search's result condition, with the constants the call supplies.
// False means no guarantee, never proof that the callee has no effect.
type CompletionSummaryLookup func(ssa.Instruction, ssa.Value, string, bool, ssaflow.CallCondition) bool

// queryAt is the question a summary lookup answers for one call: this
// search's result condition, with the constants the call supplies.
func (search *completionSearch) queryAt(instruction ssa.Instruction) ssaflow.CallCondition {
	query := search.condition
	query.Arguments = ssaflow.SuppliedConstants(ssaflow.InstructionCall(instruction), search.constants)
	return query
}

// ProveCompletionForCase summarizes exact parameter cleanup on the normal
// returns of one case: the returns matching the condition's result test, on
// the paths feasible when its assumed arguments hold. It reuses the
// completion engine and its shared budget, callback bindings, and recursion
// guard; it does not invent a caller or SSA. Without ExactTarget, a method
// completion may settle a field or element beneath the parameter; the proof
// then names that path, and a caller must not credit a claim whose path is
// not known.
func ProveCompletionForCase(function *ssa.Function, condition ssaflow.CallCondition, request CompletionRequest) ssaflow.CompletionProof {
	unknown := ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	parameter, ok := request.Target.(*ssa.Parameter)
	if !ok || function == nil || parameter.Parent() != function || len(function.Blocks) == 0 ||
		!condition.ValidFor(function.Signature) || request.InvokeTarget && len(request.Methods) != 0 {
		return unknown
	}
	// A completion case binds Boolean arguments only; a nilness condition
	// belongs to result cases, and this proof has no binding for it.
	constants, ok := condition.Arguments.Bindings(function)
	if !ok || condition.Unconditional() || condition.Nilness.Bound != 0 {
		return unknown
	}
	methods := request.Methods
	if request.InvokeTarget {
		methods = []string{""}
	}
	locals := []mappedLocal{{local: parameter, supplied: parameter, kind: localExact}}
	resultTest := ssaflow.CallCondition{Result: condition.Result, Outcome: condition.Outcome}
	for _, method := range methods {
		search := newCompletionSearch(method, CoverageEveryReturn, request.Budget)
		search.exactTarget = request.ExactTarget || request.InvokeTarget
		search.exactInvocation, search.invokeTarget = request.InvokeTarget, request.InvokeTarget
		search.summarized = request.Summarized
		search.callContract = request.CallContract
		search.returnedSummaries = request.ReturnedSummaries
		search.constants = constants
		proven := false
		paths := completionPaths{}
		search.paths = &paths
		search.memo.WithFunction(function, func() {
			if resultTest.Outcome == ssaflow.OutcomeAny {
				calls := func(candidate ssa.Instruction) bool { return search.instructionCompletes(candidate, locals, parameter) }
				proven = methodCallCoverageAssuming(function, calls, CoverageEveryReturn, ssaflow.EntryAssumptions{NonNil: parameter, Constants: constants})
				return
			}
			proven = search.conditionalCoverage(function, locals, parameter, resultTest)
		})
		if proven && !request.Budget.Exhausted() {
			return ssaflow.CompletionProof{
				Proof: ssaflow.Proof{
					State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceCalledCompletion, Method: method, Provenance: ssaflow.EvidenceFromLocalSSA,
				},
				Path: paths.path, PathKnown: paths.known(),
			}
		}
	}
	return unknown
}
