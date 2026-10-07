package path

import (
	"golang.org/x/tools/go/ssa"
)

// These probes expose the production decoders to external-package tests.
// Production consumers obtain guard identities through setup and transitions.
func GuardCondition(condition ssa.Value) (identity string, negated, stable, ok bool) {
	return guardConditionWithin(condition, nil)
}

func GuardAddressIdentity(address ssa.Value) (string, bool) {
	return guardAddressIdentityWithin(address, nil)
}

// Default-policy probes used only by tests delegate to the production engines.

// FeasibleSuccessors preserves constants selected by predecessor-sensitive
// phis and literal results of bounded, source-visible helpers. This prevents
// impossible loop exits and helper-error paths from faking leaks.
func FeasibleSuccessors(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	return FeasibleSuccessorsWithin(block, predecessor, nil)
}

// GuardsDominating collects the guards every path to target passed through:
// dominating branches one of whose arms dominates target's block. A store to
// the guarded cell inside that arm, before target, means the guard may no
// longer hold there and is not kept.
func GuardsDominating(target ssa.Instruction) PathGuards {
	return GuardsDominatingWithin(target, nil)
}

// After returns the guards that still hold once instruction has run. A store
// forgets the guards on its cell. Running a call again, as the next iteration
// of a loop does, replaces its result, so the guards on the old result no
// longer describe the new one.
func (guards PathGuards) After(instruction ssa.Instruction) PathGuards {
	return guards.AfterWithin(instruction, nil)
}

// Edges returns the feasible edges out of block for a path that arrived from
// predecessor carrying guards.
func (policy SuccessorPolicy) Edges(block, predecessor *ssa.BasicBlock, guards PathGuards) []SuccessorEdge {
	return policy.EdgesWithin(block, predecessor, guards, nil)
}
