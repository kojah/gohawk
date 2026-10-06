package lifecyclefacts

import (
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Call-retention queries bind published lifecycle claims to caller arguments.
// A visible private callee may supply local retention evidence; unavailable
// summaries remain distinct from proven absence of a claim or retained value.

// ArgumentRetainedByCallee reports whether the call's static callee is
// summarized as keeping the argument that contains target somewhere other
// than its returned value: a logger sink, a registry, a receiver field. The
// callee then owns the value's release. A parameter kept only inside the
// returned aggregate is decided by the returned-owner and view rules instead.
func (evidence *LifecycleEvidence) ArgumentRetainedByCallee(instruction ssa.Instruction, target ssa.Value) bool {
	fact, ok := factFor(evidence.pass, instruction)
	if !ok {
		return evidence.visibleCalleeRetains(instruction, target)
	}
	if !factOwnsExactArgument(instruction, target, fact.Stored()&^fact.ReturnedOwner()) {
		return false
	}
	evidence.emit(EvidenceRequest{Instruction: instruction, Target: target}, Proof{Proof: proofs.Proof{
		State: proofs.EvidenceProven, Provenance: proofs.EvidenceFromImportedFact,
	}, SummaryReason: reasonStoredByCallee})
	return true
}

// Export filtering must not make a visible private helper opaque to retention
// queries. Reuse the strict bounded classifier, but require its witness before
// every normal return instead of promoting a possible store to completion.
func (evidence *LifecycleEvidence) visibleCalleeRetains(instruction ssa.Instruction, target ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	function, closure := ssacall.DirectCallee(common)
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	retentions := evidence.retentionQueries()
	for _, binding := range ssacall.CallBindings(common, function, closure) {
		if !heapmodel.NewStorage(nil).Same(binding.Supplied, target).Proven() ||
			!retentions.storedEveryReturn(evidence.pass, function, binding.Local) {
			continue
		}
		evidence.emit(EvidenceRequest{Instruction: instruction, Target: target}, Proof{Proof: proofs.Proof{
			State: proofs.EvidenceProven, Provenance: proofs.EvidenceFromLocalSSA,
		}, SummaryReason: reasonStoredByCallee})
		return true
	}
	return false
}

// CalleeClaims reports what the call's static callee is summarized as doing
// with the argument at index. The second result separates a callee proven not
// to do it from one that carries no summary at all, which a proof must decide
// about for itself: a rule looking for evidence that an obligation was
// discharged must not read silence as proof that it was not.
func (evidence *LifecycleEvidence) CalleeClaims(
	instruction ssa.Instruction,
	index int,
	claim Claim,
) (holds bool, known bool) {
	fact, ok := factFor(evidence.pass, instruction)
	if !ok {
		return false, false
	}
	return fact.Claim(claim).contains(index), true
}

// ContentsKeptAt reports whether the call's static callee is summarized as
// possibly keeping the contents at path beneath the argument at index
// beyond the call; the empty path asks about anything inside it. The second
// result is false when the callee has no summary, which a consumer must
// treat as unknown rather than as a proof of nothing kept.
func (evidence *LifecycleEvidence) ContentsKeptAt(instruction ssa.Instruction, index int, path string) (kept bool, known bool) {
	fact, ok := factFor(evidence.pass, instruction)
	if !ok {
		return false, false
	}
	return fact.Retained().contains(index) || fact.keepsContentsAt(index, path), true
}

// CalleeSummarized reports whether the call's static callee carries a
// lifecycle summary, so a consumer can distinguish a callee proven to do
// nothing with an argument from one it knows nothing about.
func (evidence *LifecycleEvidence) CalleeSummarized(instruction ssa.Instruction) bool {
	_, ok := factFor(evidence.pass, instruction)
	return ok
}

// Declaration masks become call-site claims through exact storage identity or
// the existing guarded containment relation. Bounded queries preserve storage
// cutoff as unknown; ambiguous aliasing cannot become exact target coverage.

// factOwnsExactArgument is factOwnsArgument without containment: only the
// target itself passed as the masked argument counts, so a literal that
// captured the target is not mistaken for it.
func factOwnsExactArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask) bool {
	return factArgumentMatches(instruction, target, mask, func(value, target ssa.Value) bool {
		return heapmodel.NewStorage(nil).Same(value, target).Proven()
	})
}

func factArgumentMatches(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, matches func(ssa.Value, ssa.Value) bool) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if mask.contains(index) && matches(argument, target) {
			return true
		}
	}
	return false
}

// factOwnsArgument reports whether mask covers the argument which contains
// target at this callsite.
func factOwnsArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer proofs.Observer) bool {
	return proveFactOwnsArgumentWithin(instruction, target, mask, observer, nil).Proven()
}

func proveFactOwnsArgumentWithin(
	instruction ssa.Instruction, target ssa.Value,
	mask ParameterMask, observer proofs.Observer, budget *proofs.SearchBudget,
) proofs.Proof {
	missing := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return missing
	}
	for index, argument := range common.Args {
		if !budget.Spend() {
			return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
		}
		if !mask.contains(index) {
			continue
		}
		storageBudget := budget.Within(proofs.QueryBudget).Observed(observer)
		same := heapmodel.NewStorage(storageBudget).Same(argument, target)
		// Default callers retain their previous independent storage cap and
		// containment fallback. A bounded caller must preserve cutoff availability.
		if budget != nil && (storageBudget.Exhausted() || storageBudget.PoolExhausted()) {
			return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
		}
		if same.Proven() {
			return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk}
		}
		// Containment cannot turn an ambiguous phi or storage-history match into
		// a guarantee about this exact target. Keep the existing alias exclusion.
		if !budget.Spend() {
			return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
		}
		if !heapmodel.MayAlias(argument, target) {
			contains := lifecycle.ProveMayContainValueWithin(argument, target, budget)
			if contains.State == proofs.EvidenceUnknown || contains.Proven() {
				return contains
			}
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	return missing
}

func factOwnsProjectedArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer proofs.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		storage := heapmodel.NewStorage(proofs.NewSearchBudget(proofs.QueryBudget).Observed(observer))
		if mask.contains(index) && storage.Projection(argument, target, instruction).Proven() {
			return true
		}
	}
	return false
}
