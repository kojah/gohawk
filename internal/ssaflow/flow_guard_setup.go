package ssaflow

import (
	"slices"
	"strings"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

// Initial guard setup scans dominators and store invalidation evidence before
// the path walk starts. It shares the flow allowance through condition/address
// decoding and order checks. Interrupted setup supplies no seed; the flow must
// remain uncertain instead of judging paths from incomplete guard evidence.

// GuardsDominatingWithin shares budget across dominator visits, condition and
// address decoding, cycle checks and invalidating-store scans. At cutoff it
// returns no seed; callers inspect availability before judging return coverage.
// A nil budget retains the existing bounded guard-selection policy.
func GuardsDominatingWithin(target ssa.Instruction, budget *proofs.SearchBudget) PathGuards {
	var guards PathGuards
	block := target.Block()
	for dominator := block.Idom(); dominator != nil && len(guards) < GuardLimit; dominator = dominator.Idom() {
		if !budget.Spend() {
			return nil
		}
		if len(dominator.Succs) != 2 || len(dominator.Instrs) == 0 {
			continue
		}
		branch, ok := dominator.Instrs[len(dominator.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		identity, negated, stable, ok := guardConditionWithin(branch.Cond, budget)
		if budget.Exhausted() {
			return nil
		}
		if !ok {
			continue
		}
		if !budget.Spend() {
			return nil
		}
		taken, arm := true, dominator.Succs[0]
		if !arm.Dominates(block) {
			if !budget.Spend() {
				return nil
			}
			taken, arm = false, dominator.Succs[1]
			if !arm.Dominates(block) {
				continue
			}
		}
		stored := guardStoredWithin(identity, arm, target, budget)
		if budget.Exhausted() {
			return nil
		}
		if !stored {
			guards = append(guards, PathGuard{Identity: identity, Value: taken != negated, Stable: stable})
		}
	}
	slices.SortFunc(guards, func(left, right PathGuard) int { return strings.Compare(left.Identity, right.Identity) })
	return guards
}

func guardStoredWithin(identity string, arm *ssa.BasicBlock, target ssa.Instruction, budget *proofs.SearchBudget) bool {
	for instruction := range InstructionsWithin(target.Parent(), budget) {
		store, ok := instruction.(*ssa.Store)
		if !ok {
			continue
		}
		address, ok := guardAddressIdentityWithin(store.Addr, budget)
		if budget.Exhausted() {
			return false
		}
		if ok && strings.Contains(identity, address) && arm.Dominates(store.Block()) && InstructionMayFollowWithin(store, target, budget) {
			return true
		}
	}
	return false
}
