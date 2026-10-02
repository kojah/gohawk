package goroutineownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
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
	memo    *ssaflow.CallGraphMemo[workerReceiveKey, ssaflow.Proof]
	budget  *ssaflow.SearchBudget
	matches func(*ssa.Function, ssa.Value, ssa.Value) bool
}

func newWorkerReceiveSearch(budget *ssaflow.SearchBudget, matches func(*ssa.Function, ssa.Value, ssa.Value) bool) *workerReceiveSearch {
	return &workerReceiveSearch{
		memo: ssaflow.NewCallGraphMemo[workerReceiveKey, ssaflow.Proof](), budget: budget, matches: matches,
	}
}

func (search *workerReceiveSearch) prove(function *ssa.Function, local ssa.Value) ssaflow.Proof {
	if local == nil {
		return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}
	}
	return search.memo.Summarize(workerReceiveKey{function, local}, function, search.budget, func() ssaflow.Proof {
		return search.search(function, local)
	}, func(reason ssaflow.SummaryUnavailable, _ ssaflow.Proof) ssaflow.Proof {
		why := ssaflow.EvidenceSummaryBodyUnavailable
		switch reason {
		case ssaflow.SummaryRecursive:
			why = ssaflow.EvidenceSummaryRecursive
		case ssaflow.SummaryBudgetExhausted:
			why = ssaflow.EvidenceBudgetExhausted
		case ssaflow.SummaryBodyUnavailable:
		}
		return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: why}
	})
}

func (search *workerReceiveSearch) search(function *ssa.Function, local ssa.Value) ssaflow.Proof {
	result := ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound, Provenance: ssaflow.EvidenceFromLocalSSA}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !search.budget.Spend() {
				return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
			}
			if receivesFromWithin(instruction, func(channel ssa.Value) bool { return search.matches(function, local, channel) }, search.budget) {
				return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk, Provenance: ssaflow.EvidenceFromLocalSSA}
			}
			common := ssaflow.InstructionCall(instruction)
			if common == nil {
				continue
			}
			proof := search.throughCall(common, local)
			if proof.Proven() {
				return proof
			}
			if proof.State == ssaflow.EvidenceUnknown {
				result = proof
			}
		}
	}
	return result
}

func (search *workerReceiveSearch) throughCall(common *ssa.CallCommon, local ssa.Value) ssaflow.Proof {
	result := ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
	derives := func(value ssa.Value) bool { return heapmodel.ValueDerivesFrom(value, local) }
	callee, closure := ssaflow.DirectCallee(common)
	for _, binding := range ssaflow.CallBindings(common, callee, closure) {
		if !search.budget.Spend() {
			return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if !derives(binding.Supplied) {
			continue
		}
		proof := search.prove(callee, binding.Local)
		if proof.Proven() {
			return proof
		}
		if proof.State == ssaflow.EvidenceUnknown {
			result = proof
		}
	}
	if callee == nil {
		carried := slices.ContainsFunc(common.Args, func(argument ssa.Value) bool {
			// Stop scanning when the query is exhausted; the result below
			// distinguishes that cutoff from an actual carried argument.
			return !search.budget.Spend() || derives(argument)
		})
		if search.budget.Exhausted() {
			return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if carried {
			return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceSummaryBodyUnavailable}
		}
	}
	return result
}
