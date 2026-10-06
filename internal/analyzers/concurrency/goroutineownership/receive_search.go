package goroutineownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Worker receive searches find possible caller-owned lifetime bounds, never
// joins. The caller supplies the channel or stable context-field policy; this
// traversal owns call bindings, value-specific memoization and search limits.

type workerReceiveKey struct {
	function *ssa.Function
	local    ssa.Value
}

type workerReceiveSearch struct {
	memo    *ssacall.CallGraphMemo[workerReceiveKey, proofs.Proof]
	budget  *proofs.SearchBudget
	matches func(*ssa.Function, ssa.Value, ssa.Value) bool
}

func newWorkerReceiveSearch(budget *proofs.SearchBudget, matches func(*ssa.Function, ssa.Value, ssa.Value) bool) *workerReceiveSearch {
	return &workerReceiveSearch{
		memo: ssacall.NewCallGraphMemo[workerReceiveKey, proofs.Proof](), budget: budget, matches: matches,
	}
}

func (search *workerReceiveSearch) prove(function *ssa.Function, local ssa.Value) proofs.Proof {
	if local == nil {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}
	}
	return search.memo.Summarize(workerReceiveKey{function, local}, function, search.budget, func() proofs.Proof {
		return search.search(function, local)
	}, func(reason ssacall.SummaryUnavailable, _ proofs.Proof) proofs.Proof {
		why := proofs.EvidenceSummaryBodyUnavailable
		switch reason {
		case ssacall.SummaryRecursive:
			why = proofs.EvidenceSummaryRecursive
		case ssacall.SummaryBudgetExhausted:
			why = proofs.EvidenceBudgetExhausted
		case ssacall.SummaryBodyUnavailable:
		}
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: why}
	})
}

func (search *workerReceiveSearch) search(function *ssa.Function, local ssa.Value) proofs.Proof {
	result := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound, Provenance: proofs.EvidenceFromLocalSSA}
	for instruction := range ssaflow.InstructionsWithin(function, search.budget) {
		if receivesFromWithin(instruction, func(channel ssa.Value) bool { return search.matches(function, local, channel) }, search.budget) {
			return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA}
		}
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			continue
		}
		proof := search.throughCall(common, local)
		if proof.Proven() {
			return proof
		}
		if proof.State == proofs.EvidenceUnknown {
			result = proof
		}
	}
	// Iterator cutoff preserves the same unavailable result as a body query;
	// no incomplete negative answer may enter the memo.
	if search.budget.Exhausted() {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	return result
}

func (search *workerReceiveSearch) throughCall(common *ssa.CallCommon, local ssa.Value) proofs.Proof {
	result := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
	derives := func(value ssa.Value) bool { return heapmodel.ValueDerivesFrom(value, local) }
	callee, closure := ssacall.DirectCallee(common)
	for binding := range ssacall.CallBindingsWithin(common, callee, closure, search.budget) {
		if !derives(binding.Supplied) {
			continue
		}
		proof := search.prove(callee, binding.Local)
		if proof.Proven() {
			return proof
		}
		if proof.State == proofs.EvidenceUnknown {
			result = proof
		}
	}
	if search.budget.Exhausted() {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	if callee == nil {
		carried := slices.ContainsFunc(common.Args, func(argument ssa.Value) bool {
			// Stop scanning when the query is exhausted; the result below
			// distinguishes that cutoff from an actual carried argument.
			return !search.budget.Spend() || derives(argument)
		})
		if search.budget.Exhausted() {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
		}
		if carried {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceSummaryBodyUnavailable}
		}
	}
	return result
}
