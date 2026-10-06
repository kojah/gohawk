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

// guardConditionWithin decodes condition identity, polarity and stability.
// An interrupted decode supplies no identity or path-pruning evidence.
func guardConditionWithin(condition ssa.Value, budget *SearchBudget) (identity string, negated, stable, ok bool) {
	return guardConditionWithFormats(condition, budget, nil)
}

func guardConditionWithFormats(condition ssa.Value, budget *SearchBudget, formats *guardFormats) (identity string, negated, stable, ok bool) {
	condition, inverted := booleanNegationSourceWithin(condition, budget)
	if budget.Exhausted() {
		return "", false, false, false
	}
	identity, negated, stable, ok = guardConditionSource(condition, budget, formats)
	if budget.Exhausted() {
		return "", false, false, false
	}
	return identity, negated != inverted, stable, ok
}

func guardConditionSource(condition ssa.Value, budget *SearchBudget, formats *guardFormats) (identity string, negated, stable, ok bool) {
	if identity, negated, ok := loadedGuard(condition, budget, formats); ok {
		return identity, negated, false, true
	}
	if _, parameter := condition.(*ssa.Parameter); parameter && booleanValue(condition) {
		return fmt.Sprintf("value:%p", condition), false, true, true
	}
	if comparison, ok := condition.(*ssa.BinOp); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) &&
		stableOperandWithin(comparison.X, budget) && stableOperandWithin(comparison.Y, budget) {
		return stableEquality(comparison.X, comparison.Y), comparison.Op == token.NEQ, true, true
	}
	// A computed Boolean outside a cycle is evaluated once, so repeating that
	// exact SSA value cannot change its truth, including a short-circuit phi.
	// A loop instruction is not correlated across iterations: its next
	// evaluation may differ even though its SSA node is the same.
	// https://github.com/pb33f/libopenapi/blob/07795ddc2c097af8581138ef290d6cf964110d74/index/extract_refs_lookup.go#L199-L220
	if instruction, ok := condition.(ssa.Instruction); ok && booleanValue(condition) {
		cyclic := BlockInCycleWithin(instruction.Block(), budget)
		if !cyclic && !budget.Exhausted() {
			return fmt.Sprintf("value:%p", condition), false, true, true
		}
	}
	return "", false, false, false
}

func stableEquality(left, right ssa.Value) string {
	first, second := guardOperandIdentity(left), guardOperandIdentity(right)
	if second < first {
		first, second = second, first
	}
	return "eq(" + first + "," + second + ")"
}

func loadedGuard(condition ssa.Value, budget *SearchBudget, formats *guardFormats) (string, bool, bool) {
	switch typed := condition.(type) {
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return "", false, false
		}
		address, ok := guardAddressIdentityWithFormats(typed.X, budget, formats)
		return formats.loadedIdentity(condition, address, nil, ok), false, ok
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
		address, ok := guardAddressIdentityWithFormats(load.X, budget, formats)
		return formats.loadedIdentity(condition, address, literal, ok), typed.Op == token.NEQ, ok
	}
	return "", false, false
}

// guardAddressIdentityWithin names a cell by its selected address/load path.
func guardAddressIdentityWithin(address ssa.Value, budget *SearchBudget) (string, bool) {
	return guardAddressIdentityWithFormats(address, budget, nil)
}

func guardAddressIdentityWithFormats(address ssa.Value, budget *SearchBudget, formats *guardFormats) (string, bool) {
	if !budget.Spend() {
		return "", false
	}
	switch typed := address.(type) {
	case *ssa.Alloc, *ssa.Parameter, *ssa.FreeVar, *ssa.Global:
		return formats.addressIdentity(address, "", true), true
	case *ssa.FieldAddr:
		inner, ok := guardAddressIdentityWithFormats(typed.X, budget, formats)
		return formats.addressIdentity(address, inner, ok), ok
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			inner, ok := guardAddressIdentityWithFormats(typed.X, budget, formats)
			return formats.addressIdentity(address, inner, ok), ok
		}
	case *ssa.Call, *ssa.Extract:
		// A pointer a call returned is one object until the call runs again,
		// so two reads of the same field of it read the same slot, as two
		// reads of a parameter's field do: a response's status checked
		// twice. A walker forgets the guard when the call reruns; see After.
		return formats.addressIdentity(address, "", true), true
	}
	return "", false
}

// A call result outside a cycle is computed once per invocation, like a
// parameter, so comparing it twice compares the same value: two checks of one
// err agree.
func stableOperandWithin(value ssa.Value, budget *SearchBudget) bool {
	if !budget.Spend() {
		return false
	}
	switch value := value.(type) {
	case *ssa.Parameter, *ssa.Const:
		return true
	case *ssa.Call:
		return !BlockInCycleWithin(value.Block(), budget) && !budget.Exhausted()
	case *ssa.Extract:
		call, ok := value.Tuple.(*ssa.Call)
		return ok && !BlockInCycleWithin(call.Block(), budget) && !budget.Exhausted()
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

// ExtendWithin shares guard decoding and comparison with budget. Incomplete
// guards or contradictions are unavailable; callers must check exhaustion.
func (guards PathGuards) ExtendWithin(
	block, successor *ssa.BasicBlock, keep func(PathGuard) bool, budget *SearchBudget,
) (PathGuards, GuardContradiction) {
	return guards.extendWithFormats(block, successor, keep, budget, nil)
}

func (guards PathGuards) extendWithFormats(
	block, successor *ssa.BasicBlock, keep func(PathGuard) bool, budget *SearchBudget, formats *guardFormats,
) (PathGuards, GuardContradiction) {
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return guards, GuardConsistent
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return guards, GuardConsistent
	}
	identity, negated, stable, ok := guardConditionWithFormats(branch.Cond, budget, formats)
	if budget.Exhausted() {
		return nil, GuardConsistent
	}
	if !ok {
		return guards, GuardConsistent
	}
	guard := PathGuard{Identity: identity, Value: (successor == block.Succs[0]) != negated, Stable: stable}
	if keep != nil && !keep(guard) {
		return guards, GuardConsistent
	}
	for _, held := range guards {
		if !budget.Spend() {
			return nil, GuardConsistent
		}
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

// forgetWithin drops guards about the stored cell and paths above/beneath it.
func (guards PathGuards) forgetWithin(store *ssa.Store, budget *SearchBudget, formats *guardFormats) PathGuards {
	address, ok := guardAddressIdentityWithFormats(store.Addr, budget, formats)
	if budget.Exhausted() {
		return nil
	}
	if !ok {
		return guards
	}
	return guards.withoutIdentityWithin(address, budget)
}

// Store invalidation and rerun-result invalidation share the same exact
// identity filtering; neither may keep a partial guard list at cutoff.
func (guards PathGuards) withoutIdentityWithin(identity string, budget *SearchBudget) PathGuards {
	if len(guards) > GuardLimit {
		return guards.withoutIdentityInto(nil, identity, budget)
	}
	// Collect the bounded list on the stack,
	// then detach exactly the retained entries so sibling paths stay independent.
	var buffer [GuardLimit]PathGuard
	kept := guards.withoutIdentityInto(buffer[:0], identity, budget)
	if len(kept) == 0 {
		return nil
	}
	result := make(PathGuards, len(kept))
	copy(result, kept)
	return result
}

// withoutIdentityInto owns filtering and its charges for both bounded and
// oversized inputs. A cutoff never publishes the partially collected guards.
func (guards PathGuards) withoutIdentityInto(kept PathGuards, identity string, budget *SearchBudget) PathGuards {
	for _, guard := range guards {
		if !budget.Spend() {
			return nil
		}
		if !strings.Contains(guard.Identity, identity) {
			kept = append(kept, guard)
		}
	}
	return kept
}

// AfterWithin shares guard invalidation with budget. A cutoff cannot establish
// that the remaining guards hold; callers must check exhaustion.
func (guards PathGuards) AfterWithin(instruction ssa.Instruction, budget *SearchBudget) PathGuards {
	return guards.afterWithFormats(instruction, budget, nil)
}

func (guards PathGuards) afterWithFormats(instruction ssa.Instruction, budget *SearchBudget, formats *guardFormats) PathGuards {
	switch typed := instruction.(type) {
	case *ssa.Store:
		return guards.forgetWithin(typed, budget, formats)
	case *ssa.Call, *ssa.Extract:
		if len(guards) == 0 {
			return nil
		}
		identity := formats.addressIdentity(typed.(ssa.Value), "", true)
		return guards.withoutIdentityWithin(identity, budget)
	}
	return guards
}

// KeyWithin charges guard entries before rendering. An exhausted partial key
// must not enter a visited set; a nil budget retains the default policy.
func (guards PathGuards) KeyWithin(budget *SearchBudget) string {
	if len(guards) == 0 {
		return ""
	}
	capacity := 0
	for index, guard := range guards {
		if !budget.Spend() {
			return ""
		}
		capacity = pathGuardKeyCapacity(capacity, len(guard.Identity), guard.Value, index > 0)
	}
	var key strings.Builder
	if capacity > 0 {
		key.Grow(capacity)
	}
	for index, guard := range guards {
		if index > 0 {
			key.WriteByte(';')
		}
		key.WriteString(guard.Identity)
		key.WriteByte('=')
		if guard.Value {
			key.WriteString("true")
		} else {
			key.WriteString("false")
		}
	}
	return key.String()
}

// An oversized key disables the allocation hint; it must never wrap into a
// negative Grow argument or an incorrectly small positive capacity.
func pathGuardKeyCapacity(size, identityBytes int, value, separator bool) int {
	if size < 0 {
		return -1
	}
	suffix := len("=false")
	if value {
		suffix = len("=true")
	}
	if separator {
		suffix++
	}
	maxInt := int(^uint(0) >> 1)
	if identityBytes > maxInt-size-suffix {
		return -1
	}
	return size + identityBytes + suffix
}
