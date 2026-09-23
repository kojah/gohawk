package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestBoundedRequirements(t *testing.T) {
	makeCandidates := func() []requirementCandidate {
		candidates := make([]requirementCandidate, heapRequirementProofLimit+1)
		for index := range candidates {
			candidates[index] = requirementCandidate{
				key: requirementKey{slot: slot{region: &region{kind: regionExternal, serial: index}}},
				requirement: HeapRequirement{
					Slot: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: index}}, Kind: HeapRequiresNonNil,
				},
			}
		}
		return candidates
	}
	t.Run("published limit", func(t *testing.T) {
		calls := 0
		got := boundedRequirements(makeCandidates(), func(requirementKey) bool { calls++; return true })
		if len(got) != heapRequirementLimit || calls != heapRequirementLimit {
			t.Fatalf("got %d requirements after %d proofs, want %d of each", len(got), calls, heapRequirementLimit)
		}
		for index, requirement := range got {
			if requirement.Slot.Root.Index != index {
				t.Fatalf("requirement %d names parameter %d, want %d", index, requirement.Slot.Root.Index, index)
			}
		}
	})
	t.Run("proof-work limit", func(t *testing.T) {
		calls := 0
		got := boundedRequirements(makeCandidates(), func(key requirementKey) bool {
			calls++
			return key.slot.region.serial == heapRequirementProofLimit
		})
		if calls != heapRequirementProofLimit || len(got) != 0 {
			t.Fatalf("got %d requirements after %d proofs, want none after %d", len(got), calls, heapRequirementProofLimit)
		}
	})
}

func TestEveryReturnRequirementDeclinesBudgetCut(t *testing.T) {
	pkg := buildTestSSA(t, obligationFixture)
	function := pkg.Func("exactEverywhere")
	calls := func(instruction ssa.Instruction) bool { return labelledCall(instruction) == ObligationExact }
	if onEveryReturn(function, NewSearchBudget(0), calls) {
		t.Fatal("an exhausted path search established a requirement")
	}
	if !onEveryReturn(function, NewSearchBudget(100), calls) {
		t.Fatal("a bounded search missed calls on every return")
	}
}
