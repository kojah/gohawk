package resourcelifetime

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Path guard facts remember which way a guard went on the path that reached
// the acquisition. When the same guard is tested again later and the path
// takes the other arm, that edge is classified unknown rather than walked as
// a leak. A guard that is a load could have changed through a pointer the
// analysis does not see, so a repeated guard never prunes a path and never
// proves cleanup; it only declines to report through a contradiction the
// analysis cannot rule out. A visible store to the guarded cell forgets the
// fact. Mutagen guards both a profiler's creation and its finalization on
// one address-taken flag, and fortio both a profile file's creation and its
// close on one option field:
// https://github.com/mutagen-io/mutagen/blob/6ccfeaaf4dfd261e59ef9aac56e3c157b62e605b/tools/scan_bench/main.go#L140-L172
// https://github.com/fortio/fortio/blob/5c19725ff61c9f7ad944b91ec32d96a399341d87/fhttp/httprunner.go#L199-L215

// resourceGuardLimit bounds the facts one path carries so the state space
// stays small; a fifth guard is simply not remembered.
const resourceGuardLimit = 4

type resourceGuard struct {
	identity string
	value    bool
}

func resourceGuardKey(guards []resourceGuard) string {
	parts := make([]string, 0, len(guards))
	for _, guard := range guards {
		parts = append(parts, fmt.Sprintf("%s=%t", guard.identity, guard.value))
	}
	return strings.Join(parts, ";")
}

// resourceGuardCondition decodes an If condition into a stable identity and
// whether taking the true arm makes the fact false, as a != comparison does.
// A loaded cell or field is identified by its access path, so two distinct
// loads of the same place share an identity; a Boolean parameter or a Boolean
// computed outside any cycle is identified by its SSA value, which is
// evaluated once. Anything else has no identity.
func resourceGuardCondition(condition ssa.Value) (string, bool, bool) {
	if identity, negated, ok := loadedGuardCondition(condition); ok {
		return identity, negated, true
	}
	if _, parameter := condition.(*ssa.Parameter); parameter && booleanTyped(condition) {
		return fmt.Sprintf("value:%p", condition), false, true
	}
	if instruction, ok := condition.(ssa.Instruction); ok && booleanTyped(condition) && !ssaflow.BlockInCycle(instruction.Block()) {
		return fmt.Sprintf("value:%p", condition), false, true
	}
	return "", false, false
}

func loadedGuardCondition(condition ssa.Value) (string, bool, bool) {
	switch typed := condition.(type) {
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return "", false, false
		}
		address, ok := guardAddressIdentity(typed.X)
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
		address, ok := guardAddressIdentity(load.X)
		return "eq(load(" + address + ")," + guardConstantIdentity(literal) + ")", typed.Op == token.NEQ, ok
	}
	return "", false, false
}

// guardAddressIdentity names a cell by the path that reaches it: a local
// allocation, a parameter, a captured variable, a package variable, or a field
// selected from one of those, possibly through a loaded pointer.
func guardAddressIdentity(address ssa.Value) (string, bool) {
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
		inner, ok := guardAddressIdentity(typed.X)
		return fmt.Sprintf("field(%s,%d)", inner, typed.Field), ok
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			inner, ok := guardAddressIdentity(typed.X)
			return "load(" + inner + ")", ok
		}
	}
	return "", false
}

func guardConstantIdentity(literal *ssa.Const) string {
	if literal.Value == nil {
		return "nil"
	}
	return literal.Value.ExactString()
}

func booleanTyped(value ssa.Value) bool {
	basic, ok := value.Type().Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsBoolean != 0
}

// guardsAtAcquisition collects the guards every path to the acquisition
// passed through: dominating branches one of whose arms dominates the
// acquisition's block. A store to the guarded cell inside that arm, before
// the acquisition, means the fact may no longer hold there and is not kept.
func (analysis *resourceAnalysis) guardsAtAcquisition() []resourceGuard {
	var guards []resourceGuard
	block := analysis.acquisition.Block()
	for dominator := block.Idom(); dominator != nil && len(guards) < resourceGuardLimit; dominator = dominator.Idom() {
		if len(dominator.Succs) != 2 || len(dominator.Instrs) == 0 {
			continue
		}
		branch, ok := dominator.Instrs[len(dominator.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		identity, negated, ok := resourceGuardCondition(branch.Cond)
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
		if guardStoredWithin(identity, arm, analysis.acquisition) {
			continue
		}
		guards = append(guards, resourceGuard{identity: identity, value: taken != negated})
	}
	slices.SortFunc(guards, func(left, right resourceGuard) int { return strings.Compare(left.identity, right.identity) })
	return guards
}

func guardStoredWithin(identity string, arm *ssa.BasicBlock, acquisition ssa.Instruction) bool {
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](acquisition.Parent()) {
		address, ok := guardAddressIdentity(store.Addr)
		if ok && strings.Contains(identity, address) && arm.Dominates(store.Block()) && ssaflow.InstructionMayFollow(store, acquisition) {
			return true
		}
	}
	return false
}

// extendResourceGuards records the guard the edge from block to successor
// establishes. It reports a conflict when the path already holds the same
// guard with the opposite value: that edge contradicts the path it is on.
func extendResourceGuards(guards []resourceGuard, block, successor *ssa.BasicBlock) ([]resourceGuard, bool) {
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return guards, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return guards, false
	}
	identity, negated, ok := resourceGuardCondition(branch.Cond)
	if !ok {
		return guards, false
	}
	value := (successor == block.Succs[0]) != negated
	for _, guard := range guards {
		if guard.identity == identity {
			return guards, guard.value != value
		}
	}
	if len(guards) >= resourceGuardLimit {
		return guards, false
	}
	next := append(slices.Clone(guards), resourceGuard{identity: identity, value: value})
	slices.SortFunc(next, func(left, right resourceGuard) int { return strings.Compare(left.identity, right.identity) })
	return next, false
}

// forgetStoredGuards drops every fact about a cell the store may change: the
// stored place itself and any path selected beneath or above it.
func forgetStoredGuards(guards []resourceGuard, store *ssa.Store) []resourceGuard {
	address, ok := guardAddressIdentity(store.Addr)
	if !ok {
		return guards
	}
	var kept []resourceGuard
	for _, guard := range guards {
		if !strings.Contains(guard.identity, address) {
			kept = append(kept, guard)
		}
	}
	return kept
}
