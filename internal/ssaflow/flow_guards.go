package ssaflow

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Path guards remember which way a branch went on the path being walked, so
// that a later branch on the same condition can be related to it. Two kinds
// of condition have an identity. A stable one compares parameters and
// constants, or is a Boolean computed once outside any cycle: its value
// cannot change within the invocation, so a path that takes the other arm
// later is infeasible. A loaded one reads a cell through an address path:
// a store the analysis does not see could change it, so a later
// contradiction is uncertainty, never proof that the path cannot run. The
// engine records both kinds and reports which kind a contradiction is; each
// walk decides what to do with it, because pruning a path and calling it
// unknown are different claims with different risks.
//
// Two representative shapes, from opposite sides of the same policy:
// mutagen guards a profiler's creation and its finalization on one
// address-taken flag, and geesefs locks and unlocks under one five-way
// disjunction of pointer parameters.
// https://github.com/mutagen-io/mutagen/blob/6ccfeaaf4dfd261e59ef9aac56e3c157b62e605b/tools/scan_bench/main.go#L140-L172
// https://github.com/yandex-cloud/geesefs/blob/dd847771b29b26f3246edaf3227acbc430f4548d/core/file.go#L1901-L1972

// GuardLimit bounds the guards one path carries so the state space stays
// small; a guard beyond the limit is simply not remembered, which loses a
// correlation but never invents one.
const GuardLimit = 8

// PathGuard is one branch outcome the path established.
type PathGuard struct {
	Identity string
	Value    bool
	// Stable is true for a guard whose value cannot change within the
	// invocation; false for one read from a cell.
	Stable bool
}

// PathGuards is the sorted, bounded set of guards a path carries.
type PathGuards []PathGuard

// GuardContradiction is how an edge relates to the guards already held.
type GuardContradiction uint8

const (
	// GuardConsistent: the edge agrees with, or is unrelated to, every guard.
	GuardConsistent GuardContradiction = iota
	// GuardStableContradiction: the edge takes the other arm of a stable
	// guard, so no execution reaches it on this path.
	GuardStableContradiction
	// GuardLoadedContradiction: the edge takes the other arm of a loaded
	// guard, which a hidden store could explain; the edge is uncertain.
	GuardLoadedContradiction
)

// GuardCondition decodes a branch condition into a guard identity, whether
// the true arm makes the guard false (as a != comparison does), and whether
// the guard is stable. A condition with no identity reports false.
func GuardCondition(condition ssa.Value) (identity string, negated, stable, ok bool) {
	// !x tests x with the arms swapped.
	if negation, isNot := condition.(*ssa.UnOp); isNot && negation.Op == token.NOT {
		identity, negated, stable, ok = GuardCondition(negation.X)
		return identity, !negated, stable, ok
	}
	if identity, negated, ok := loadedGuard(condition); ok {
		return identity, negated, false, true
	}
	if _, parameter := condition.(*ssa.Parameter); parameter && booleanValue(condition) {
		return fmt.Sprintf("value:%p", condition), false, true, true
	}
	if comparison, ok := condition.(*ssa.BinOp); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) &&
		stableOperand(comparison.X) && stableOperand(comparison.Y) {
		left, right := guardOperandIdentity(comparison.X), guardOperandIdentity(comparison.Y)
		if right < left {
			left, right = right, left
		}
		return "eq(" + left + "," + right + ")", comparison.Op == token.NEQ, true, true
	}
	// A computed Boolean outside a cycle is evaluated once, so repeating that
	// exact SSA value cannot change its truth, including a short-circuit phi.
	// A loop instruction is not correlated across iterations: its next
	// evaluation may differ even though its SSA node is the same.
	// https://github.com/pb33f/libopenapi/blob/07795ddc2c097af8581138ef290d6cf964110d74/index/extract_refs_lookup.go#L199-L220
	if instruction, ok := condition.(ssa.Instruction); ok && booleanValue(condition) && !BlockInCycle(instruction.Block()) {
		return fmt.Sprintf("value:%p", condition), false, true, true
	}
	return "", false, false, false
}

func loadedGuard(condition ssa.Value) (string, bool, bool) {
	switch typed := condition.(type) {
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return "", false, false
		}
		address, ok := GuardAddressIdentity(typed.X)
		return "load(" + address + ")", false, ok
	case *ssa.BinOp:
		if typed.Op != token.EQL && typed.Op != token.NEQ {
			return "", false, false
		}
		left, right := typed.X, typed.Y
		if _, ok := left.(*ssa.Const); ok {
			left, right = right, left
		}
		literal, ok := right.(*ssa.Const)
		load, loaded := left.(*ssa.UnOp)
		if !ok || !loaded || load.Op != token.MUL {
			return "", false, false
		}
		address, ok := GuardAddressIdentity(load.X)
		return "eq(load(" + address + ")," + guardOperandIdentity(literal) + ")", typed.Op == token.NEQ, ok
	}
	return "", false, false
}

// GuardAddressIdentity names a cell by the path that reaches it: a local
// allocation, a parameter, a captured variable, a package variable, or a
// field selected from one of those, possibly through a loaded pointer.
func GuardAddressIdentity(address ssa.Value) (string, bool) {
	switch typed := address.(type) {
	case *ssa.Alloc:
		return fmt.Sprintf("alloc:%p", typed), true
	case *ssa.Parameter:
		return fmt.Sprintf("param:%p", typed), true
	case *ssa.FreeVar:
		return fmt.Sprintf("free:%p", typed), true
	case *ssa.Global:
		return "global:" + typed.String(), true
	case *ssa.FieldAddr:
		inner, ok := GuardAddressIdentity(typed.X)
		return fmt.Sprintf("field(%s,%d)", inner, typed.Field), ok
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			inner, ok := GuardAddressIdentity(typed.X)
			return "load(" + inner + ")", ok
		}
	}
	return "", false
}

// A call result outside a cycle is computed once per invocation, like a
// parameter, so comparing it twice compares the same value: two checks of one
// err agree.
func stableOperand(value ssa.Value) bool {
	switch value := value.(type) {
	case *ssa.Parameter, *ssa.Const:
		return true
	case *ssa.Call:
		return !BlockInCycle(value.Block())
	case *ssa.Extract:
		call, ok := value.Tuple.(*ssa.Call)
		return ok && !BlockInCycle(call.Block())
	}
	return false
}

func guardOperandIdentity(value ssa.Value) string {
	switch typed := value.(type) {
	case *ssa.Parameter:
		return fmt.Sprintf("param:%p", typed)
	case *ssa.Call, *ssa.Extract:
		return fmt.Sprintf("result:%p", typed)
	case *ssa.Const:
		if typed.Value == nil {
			return "nil:" + types.TypeString(typed.Type(), nil)
		}
		return typed.Value.ExactString()
	}
	return fmt.Sprintf("%T:%p", value, value)
}

func booleanValue(value ssa.Value) bool {
	basic, ok := value.Type().Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsBoolean != 0
}

// Extend records the guard the edge from block to successor establishes and
// reports whether it contradicts a guard the path already holds. keep, when
// set, filters which guards are remembered; a filtered-out guard is neither
// stored nor checked.
func (guards PathGuards) Extend(block, successor *ssa.BasicBlock, keep func(PathGuard) bool) (PathGuards, GuardContradiction) {
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return guards, GuardConsistent
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return guards, GuardConsistent
	}
	identity, negated, stable, ok := GuardCondition(branch.Cond)
	if !ok {
		return guards, GuardConsistent
	}
	guard := PathGuard{Identity: identity, Value: (successor == block.Succs[0]) != negated, Stable: stable}
	if keep != nil && !keep(guard) {
		return guards, GuardConsistent
	}
	for _, held := range guards {
		if held.Identity != identity {
			continue
		}
		if held.Value == guard.Value {
			return guards, GuardConsistent
		}
		if stable {
			return guards, GuardStableContradiction
		}
		return guards, GuardLoadedContradiction
	}
	if len(guards) >= GuardLimit {
		return guards, GuardConsistent
	}
	next := append(slices.Clone(guards), guard)
	slices.SortFunc(next, func(left, right PathGuard) int { return strings.Compare(left.Identity, right.Identity) })
	return next, GuardConsistent
}

// Forget drops every guard about a cell the store may change: the stored
// place itself and any path selected beneath or above it.
func (guards PathGuards) Forget(store *ssa.Store) PathGuards {
	address, ok := GuardAddressIdentity(store.Addr)
	if !ok {
		return guards
	}
	var kept PathGuards
	for _, guard := range guards {
		if !strings.Contains(guard.Identity, address) {
			kept = append(kept, guard)
		}
	}
	return kept
}

// Key renders the guards for a walk's state key.
func (guards PathGuards) Key() string {
	parts := make([]string, 0, len(guards))
	for _, guard := range guards {
		parts = append(parts, fmt.Sprintf("%s=%t", guard.Identity, guard.Value))
	}
	return strings.Join(parts, ";")
}

// GuardsDominating collects the guards every path to target passed through:
// dominating branches one of whose arms dominates target's block. A store to
// the guarded cell inside that arm, before target, means the guard may no
// longer hold there and is not kept.
func GuardsDominating(target ssa.Instruction) PathGuards {
	var guards PathGuards
	block := target.Block()
	for dominator := block.Idom(); dominator != nil && len(guards) < GuardLimit; dominator = dominator.Idom() {
		if len(dominator.Succs) != 2 || len(dominator.Instrs) == 0 {
			continue
		}
		branch, ok := dominator.Instrs[len(dominator.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		identity, negated, stable, ok := GuardCondition(branch.Cond)
		if !ok {
			continue
		}
		taken, arm := true, dominator.Succs[0]
		if !arm.Dominates(block) {
			taken, arm = false, dominator.Succs[1]
			if !arm.Dominates(block) {
				continue
			}
		}
		if guardStoredWithin(identity, arm, target) {
			continue
		}
		guards = append(guards, PathGuard{Identity: identity, Value: taken != negated, Stable: stable})
	}
	slices.SortFunc(guards, func(left, right PathGuard) int { return strings.Compare(left.Identity, right.Identity) })
	return guards
}

func guardStoredWithin(identity string, arm *ssa.BasicBlock, target ssa.Instruction) bool {
	for _, store := range InstructionsOf[*ssa.Store](target.Parent()) {
		address, ok := GuardAddressIdentity(store.Addr)
		if ok && strings.Contains(identity, address) && arm.Dominates(store.Block()) && InstructionMayFollow(store, target) {
			return true
		}
	}
	return false
}
