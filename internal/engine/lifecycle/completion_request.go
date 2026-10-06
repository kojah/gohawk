package lifecycle

import (
	"strings"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// CompletionRequest asks whether the callee launched by Instruction calls one
// of Methods on Target. The instruction's launch form decides the coverage
// the call must have; callers that accept only some launch forms, such as
// deferred releases, select the instructions they submit.
type CompletionRequest struct {
	Instruction ssa.Instruction
	Target      ssa.Value
	Methods     []string
	// InvokeTarget asks whether the exact function value Target is invoked,
	// rather than a method on an object. Methods must be empty. Asynchronous
	// launches are not invocation completion; their ownership is caller policy.
	InvokeTarget bool
	// ExactTarget excludes containment and may-alias receiver mappings. It is
	// appropriate when completion will justify cleanup in another scope.
	ExactTarget bool
	// Coverage defaults to CoverageEveryReturn. Callers asking only whether a
	// callee may complete the target select CoverageAnywhere.
	Coverage CompletionCoverage
	// Budget, when set, bounds the instructions this question may examine. A
	// nil budget leaves the search unbounded. Exhaustion is reported as an
	// Unknown proof with EvidenceBudgetExhausted rather than a disproof,
	// because the search stopped before it could decide; what an undecided
	// answer permits is the caller's policy, not this package's.
	Budget *proofs.SearchBudget
	// Summarized supplies exact completion guarantees for unavailable bodies.
	// Its policy is fixed for this request and all nested summary queries.
	Summarized CompletionSummaryLookup
	// CallContract supplies exact positive effects of a call even when its
	// body is visible. This lets external API semantics compose through a
	// local forwarding wrapper without treating missing effects as absence.
	// Its policy belongs to this request; LocalEvidence does not memoize it
	// across requests because callback identities cannot form a stable key.
	CallContract CompletionSummaryLookup
	// ReturnedSummaries supplies exact callback-to-parameter/result relations
	// for factories whose bodies are unavailable.
	ReturnedSummaries ReturnedCleanupLookup
	// Constants, when set, fixes Boolean parameters of the body containing
	// Instruction, as when that body is itself proved under one of its own
	// cases; a helper then sees the constants its call forwards.
	Constants ssacall.FixedValues
	// condition is set only by the edge query after resolving an exact call
	// result. It never changes an ordinary completion request's contract.
	condition ssacall.CallCondition
}

// ProveCompletion answers one completion request. Each call runs its own
// search, which memoizes the questions it asks itself; nothing is cached
// between requests. The proof is Unknown when no callee body was available or
// callback resolution was incomplete, so callers may consult imported
// summaries. A fully searched body that does not complete the target is
// Disproven.
func ProveCompletion(request CompletionRequest) proofs.CompletionProof {
	if request.Instruction == nil || request.Target == nil {
		return proofs.CompletionProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	if request.InvokeTarget && len(request.Methods) != 0 || !request.InvokeTarget && len(request.Methods) == 0 {
		return proofs.CompletionProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	searched := false
	incomplete := false
	inCycle := false
	methods := request.Methods
	if request.InvokeTarget {
		methods = []string{""}
	}
	for _, method := range methods {
		search := newCompletionSearch(method, request.Coverage, request.Budget)
		search.exactInvocation = request.InvokeTarget
		search.exactTarget = request.ExactTarget || request.InvokeTarget
		search.invokeTarget = request.InvokeTarget
		search.condition = request.condition
		search.constants = request.Constants
		search.summarized = request.Summarized
		search.callContract = request.CallContract
		search.returnedSummaries = request.ReturnedSummaries
		answer := search.completes(request.Instruction, request.Target)
		if answer.proven {
			return answer.proof(method)
		}
		searched = searched || answer.available
		incomplete = incomplete || *search.incomplete
		inCycle = inCycle || *search.inCycle
	}
	return request.unprovenCompletion(searched, incomplete, inCycle)
}

func (request CompletionRequest) unprovenCompletion(searched, incomplete, inCycle bool) proofs.CompletionProof {
	proof := proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}
	if searched {
		proof.Provenance = proofs.EvidenceFromLocalSSA
	}
	switch {
	case request.Budget.Exhausted():
		// The walk stopped early, so a missing completion is not evidence
		// that the callee fails to complete the target.
		proof.Reason = proofs.EvidenceBudgetExhausted
	case searched && inCycle:
		// A helper that releases every element of what it was handed inside
		// a loop, as slackdump's Destroy closes each stored handle, is not
		// covered on every return: the loop's exit edge skips the body, and
		// which element an iteration settles is decided by iteration. That
		// is uncertainty about the element, not a missing completion.
		// https://github.com/rusq/slackdump/blob/f7319928b0993b23d7e9bd8af5e4c69b6f1d2af4/internal/chunk/filemgr.go#L66-L73
		proof.Reason = proofs.EvidenceCompletionInCycle
	case searched && incomplete:
		// Unresolved nested work cannot establish missing completion.
	case searched:
		proof.State, proof.Reason = proofs.EvidenceDisproven, proofs.EvidenceNotFound
	}
	return request.giveUp(proofs.CompletionProof{Proof: proof})
}

// giveUp reports a completion search that proved nothing to the budget's
// observer, naming the launch site, the target, and the methods sought, and
// returns the proof unchanged.
func (request CompletionRequest) giveUp(proof proofs.CompletionProof) proofs.CompletionProof {
	request.Budget.Observe(proof.Reason, request.Instruction.Pos(), func() map[string]string {
		details := map[string]string{"instruction": request.Instruction.String(), "target": request.Target.Name()}
		if len(request.Methods) != 0 {
			details["methods"] = strings.Join(request.Methods, ",")
		}
		return details
	})
	return proof
}

// ProveMethodCallCoverageWithin shares witness and normal-return coverage work
// with budget. A cutoff supplies unknown, never completed coverage or its absence.
// Callbacks should use the same budget. Nil retains the default unbounded walk.
func ProveMethodCallCoverageWithin(
	function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage, nonNil ssa.Value, budget *proofs.SearchBudget,
) proofs.Proof {
	return proveMethodCallCoverageAssumingWithin(function, calls, coverage, ssapath.EntryAssumptions{NonNil: nonNil}, budget)
}

// Constant-bound census must finish before its blocks become witness evidence.
// The caller retains one budget through that census and the coverage walk.
func proveMethodCallCoverageAssumingWithin(
	function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage,
	assumptions ssapath.EntryAssumptions, budget *proofs.SearchBudget,
) proofs.Proof {
	if function == nil || len(function.Blocks) == 0 {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}
	}
	blocks := function.Blocks
	if len(assumptions.Constants) != 0 {
		blocks = ssapath.ReachableBlocksAssumingWithin(function, assumptions.Constants, budget)
		if budget.Exhausted() {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
		}
	}
	return proveMethodCoverageWithin(function, blocks, calls, coverage, assumptions, budget)
}

// Keep the independent return/action witness contract of ordinary coverage.
// The shared obligation walk then proves ordering on feasible return paths;
// witnessing an action on an unrelated path alone cannot prove completion.
func proveMethodCoverageWithin(
	function *ssa.Function, blocks []*ssa.BasicBlock, calls func(ssa.Instruction) bool,
	coverage CompletionCoverage, assumptions ssapath.EntryAssumptions, budget *proofs.SearchBudget,
) proofs.Proof {
	missing := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceUnavailable}
	cut := proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	hasReturn, hasAction := false, false
	for _, block := range blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return cut
			}
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
			if !hasAction {
				hasAction = calls(instruction)
			}
			if budget.Exhausted() {
				return cut
			}
			if coverage == CoverageAnywhere && hasAction {
				return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceCalledCompletion}
			}
		}
	}
	if !hasReturn || !hasAction {
		return missing
	}
	// A completed census prevents vacuous success on no-return/no-action bodies.
	// Budget uncertainty from either the flow or its predicate cannot settle it.
	return proveMethodReturnCoverageWithin(function, calls, assumptions, budget)
}

// This is the one feasible-return walk, shared by ordinary witness coverage
// and the existing exact-type path. Predicate and CFG work share the allowance.
func proveMethodReturnCoverageWithin(
	function *ssa.Function, calls func(ssa.Instruction) bool, assumptions ssapath.EntryAssumptions, budget *proofs.SearchBudget,
) proofs.Proof {
	cut := proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	missing := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceUnavailable}
	outcome := ssapath.EvaluateObligationFromEntry(function, ssapath.ObligationFlow{
		Budget: budget, NonNil: assumptions.NonNil, NonNilType: assumptions.NonNilType, Constants: assumptions.Constants,
		Instruction: ssapath.ExactOrNone(calls),
	})
	if budget.Exhausted() || outcome == ssapath.ObligationUncertain {
		return cut
	}
	if outcome != ssapath.ObligationHonored {
		return missing
	}
	return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceCalledCompletion}
}
