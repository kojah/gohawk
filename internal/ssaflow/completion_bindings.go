package ssaflow

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Callback bindings preserve direct function arguments while the existing
// completion search enters a visible helper. They resolve identity only:
// ordinary completion coverage still decides whether the call must happen.
// No caller enumeration, heap propagation, or cross-package facts are needed.
type callbackBindings struct {
	values map[ssa.Value]callbackValue
}

type callbackValue struct {
	value       ssa.Value
	bindings    *callbackBindings
	observation ssa.Instruction
}

func (search *completionSearch) invokesTargetLocal(value, local ssa.Value) bool {
	if !search.exactTarget {
		return invokesLocal(value, local)
	}
	if value == local {
		return true
	}
	// Only a captured cell whose stability was checked when mapping the
	// closure may be loaded here. A phi that merely includes the target is not
	// identity and cannot establish that the target was invoked.
	_, captured := local.(*ssa.FreeVar)
	load, ok := value.(*ssa.UnOp)
	return captured && ok && load.Op == token.MUL && load.X == local
}

func (search *completionSearch) bindCallbackArguments(callee completionCallee) *callbackBindings {
	bindings := &callbackBindings{values: make(map[ssa.Value]callbackValue)}
	for index, parameter := range callee.function.Params {
		if callee.common == nil || index >= len(callee.common.Args) || !search.budget.Spend() {
			break
		}
		bindings.values[parameter] = callbackValue{callee.common.Args[index], search.bindings, callee.invocation}
	}
	for _, captured := range ClosureBindingPairs(callee.function, callee.closure) {
		bindings.values[captured.Free] = callbackValue{captured.Binding, callee.environment, callee.invocation}
	}
	return bindings
}

func (search *completionSearch) boundCallees(instruction ssa.Instruction) ([]completionCallee, bool) {
	common := InstructionCall(instruction)
	if common == nil || common.IsInvoke() || common.StaticCallee() != nil || search.bindings == nil {
		return resolveCallees(instruction)
	}
	value, ok := resolveCallbackValue(callbackValue{common.Value, search.bindings, instruction}, search.budget)
	if !ok {
		if common.StaticCallee() == nil {
			*search.incomplete = true
		}
		return resolveCallees(instruction)
	}
	// Keep the invocation's arguments in its own SSA scope. Only its function
	// value is substituted; mappedLocals maps those arguments into the resolved
	// callback. Rewriting the shared SSA would contaminate other call sites.
	resolved := *common
	resolved.Value = value.value
	var callees []completionCallee
	switch instruction.(type) {
	case *ssa.Call:
		callees, ok = calleesOf(&resolved, launchCalled, instruction, false)
	case *ssa.Defer:
		callees, ok = calleesOf(&resolved, launchDeferred, instruction, true)
	case *ssa.Go:
		callees, ok = calleesOf(&resolved, launchStarted, instruction, false)
	}
	for i := range callees {
		callees[i].environment = value.bindings
	}
	return callees, ok
}

// Resolve only immutable values or a cell initialized once before observation.
// Capturing a cell does not make its contents immutable: reject reassignment
// and opaque address escape rather than using the value at registration time.
func resolveCallbackValue(ref callbackValue, budget *SearchBudget) (callbackValue, bool) {
	walk := NewReachingWalk(TransparentChangeType | TransparentConvert)
	return resolveCallbackBinding(walk, ref, budget)
}

func resolveCallbackBinding(walk ReachingWalk, ref callbackValue, budget *SearchBudget) (callbackValue, bool) {
	if ref.value == nil || !budget.Spend() || !walk.Mark(ref.value) {
		return callbackValue{}, false
	}
	if ref.bindings != nil {
		if next, ok := ref.bindings.values[ref.value]; ok {
			return resolveCallbackBinding(walk, next, budget)
		}
	}
	if inner, ok := UnwrapTransparentValue(ref.value, TransparentChangeType|TransparentConvert); ok {
		ref.value = inner
		return resolveCallbackBinding(walk, ref, budget)
	}
	switch value := ref.value.(type) {
	case *ssa.Function, *ssa.MakeClosure:
		return ref, true
	case *ssa.UnOp:
		if value.Op == token.MUL {
			if field, ok := value.X.(*ssa.FieldAddr); ok {
				return resolveCallbackField(walk, ref, field, budget)
			}
			if index, ok := value.X.(*ssa.IndexAddr); ok {
				return resolveCallbackElement(walk, ref, index, budget)
			}
			ref.value = value.X
			return resolveCallbackBinding(walk, ref, budget)
		}
	case *ssa.Alloc:
		if stored, ok := immutableCallbackCell(value, ref.observation, budget); ok {
			ref.value = stored
			return resolveCallbackBinding(walk, ref, budget)
		}
	}
	return callbackValue{}, false
}

// A projection is resolved only through an unchanged local aggregate. Unknown
// calls receiving its address can mutate a field without an SSA Store here.
func callbackAggregate(ref callbackValue, budget *SearchBudget) (callbackValue, bool) {
	for budget.Spend() {
		if ref.bindings != nil {
			if next, ok := ref.bindings.values[ref.value]; ok {
				if !callbackAggregateReadOnly(ref.value, budget) {
					return callbackValue{}, false
				}
				ref = next
				continue
			}
		}
		if slice, ok := ref.value.(*ssa.Slice); ok {
			if slice.Low != nil || slice.High != nil || slice.Max != nil {
				return callbackValue{}, false
			}
			ref.value = slice.X
			continue
		}
		_, ok := ref.value.(*ssa.Alloc)
		return ref, ok
	}
	return callbackValue{}, false
}

// The caller's allocation may look immutable while a callee writes through
// its parameter. Admit only direct reads of fields/elements in that callee.
func callbackAggregateReadOnly(value ssa.Value, budget *SearchBudget) bool {
	if value.Referrers() == nil {
		return false
	}
	for _, use := range *value.Referrers() {
		if !budget.Spend() {
			return false
		}
		var address ssa.Value
		switch use := use.(type) {
		case *ssa.FieldAddr:
			address = use
		case *ssa.IndexAddr:
			address = use
		case *ssa.Call:
			builtin, ok := use.Common().Value.(*ssa.Builtin)
			if !ok || builtin.Name() != "len" {
				return false
			}
			continue
		default:
			return false
		}
		if address.Referrers() == nil {
			return false
		}
		for _, access := range *address.Referrers() {
			if !budget.Spend() {
				return false
			}
			load, ok := access.(*ssa.UnOp)
			if !ok || load.Op != token.MUL {
				return false
			}
		}
	}
	return true
}

func resolveCallbackField(walk ReachingWalk, ref callbackValue, field *ssa.FieldAddr, budget *SearchBudget) (callbackValue, bool) {
	ref.value = field.X
	root, ok := callbackAggregate(ref, budget)
	if !ok || root.value.Referrers() == nil {
		return callbackValue{}, false
	}
	var stored ssa.Value
	for _, use := range *root.value.Referrers() {
		if !budget.Spend() {
			return callbackValue{}, false
		}
		address, ok := use.(*ssa.FieldAddr)
		if !ok {
			if use == root.observation {
				continue
			}
			return callbackValue{}, false
		}
		if address.Field != field.Field {
			continue
		}
		value, ok := immutableCallbackAddress(address, root.observation, budget)
		if !ok || stored != nil && stored != value {
			return callbackValue{}, false
		}
		stored = value
	}
	root.value = stored
	return resolveCallbackBinding(walk, root, budget)
}

func resolveCallbackElement(walk ReachingWalk, ref callbackValue, index *ssa.IndexAddr, budget *SearchBudget) (callbackValue, bool) {
	ref.value = index.X
	root, ok := callbackAggregate(ref, budget)
	if !ok || root.value.Referrers() == nil {
		return callbackValue{}, false
	}
	selected, fixed := index.Index.(*ssa.Const)
	array := callbackArray(root.value)
	if array == nil {
		return callbackValue{}, false
	}
	indices := make(map[int64]bool)
	var stored ssa.Value
	for _, use := range *root.value.Referrers() {
		if !budget.Spend() {
			return callbackValue{}, false
		}
		address, ok := use.(*ssa.IndexAddr)
		if !ok {
			if callbackSliceOnlyObserved(use, root.observation, budget) {
				continue
			}
			return callbackValue{}, false
		}
		key, known := address.Index.(*ssa.Const)
		if !known {
			return callbackValue{}, false
		}
		position, valid := constant.Int64Val(key.Value)
		if !valid || position < 0 || position >= array.Len() || indices[position] {
			return callbackValue{}, false
		}
		indices[position] = true
		if fixed && !constant.Compare(selected.Value, token.EQL, key.Value) {
			continue
		}
		value, ok := immutableCallbackAddress(address, root.observation, budget)
		if !ok || stored != nil && stored != value {
			return callbackValue{}, false
		}
		stored = value
	}
	if !fixed && int64(len(indices)) != array.Len() {
		return callbackValue{}, false
	}
	root.value = stored
	return resolveCallbackBinding(walk, root, budget)
}

func callbackArray(value ssa.Value) *types.Array {
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	array, _ := pointer.Elem().Underlying().(*types.Array)
	return array
}

func callbackSliceOnlyObserved(use, observation ssa.Instruction, budget *SearchBudget) bool {
	slice, ok := use.(*ssa.Slice)
	if !ok || slice.Referrers() == nil {
		return false
	}
	for _, consumer := range *slice.Referrers() {
		if !budget.Spend() || consumer != observation {
			return false
		}
	}
	return true
}

func immutableCallbackAddress(address ssa.Value, observation ssa.Instruction, budget *SearchBudget) (ssa.Value, bool) {
	if address.Referrers() == nil || observation == nil {
		return nil, false
	}
	var stored *ssa.Store
	for _, use := range *address.Referrers() {
		if !budget.Spend() {
			return nil, false
		}
		if store, ok := use.(*ssa.Store); ok && store.Addr == address {
			if stored != nil || !InstructionDominates(store, observation) {
				return nil, false
			}
			stored = store
		} else if load, ok := use.(*ssa.UnOp); !ok || load.Op != token.MUL {
			return nil, false
		}
	}
	if stored == nil {
		return nil, false
	}
	return stored.Val, true
}

func immutableCallbackCell(cell *ssa.Alloc, observation ssa.Instruction, budget *SearchBudget) (ssa.Value, bool) {
	if cell.Referrers() == nil || observation == nil {
		return nil, false
	}
	var stored *ssa.Store
	for _, use := range *cell.Referrers() {
		if !budget.Spend() {
			return nil, false
		}
		switch use := use.(type) {
		case *ssa.Store:
			if stored != nil || use.Addr != cell || !InstructionDominates(use, observation) {
				return nil, false
			}
			// One SSA store may execute repeatedly. A cell allocated outside
			// that loop is not immutable across its captured callbacks.
			if BlockInCycle(use.Block()) && use.Block() != cell.Block() {
				return nil, false
			}
			stored = use
		case *ssa.UnOp:
			if use.Op != token.MUL {
				return nil, false
			}
		case *ssa.MakeClosure:
			// The closure may only read this cell. Any write through a captured
			// alias is deliberately outside this initial stable-storage model.
			if !callbackCaptureReadOnly(use, cell, budget) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if stored == nil {
		return nil, false
	}
	return stored.Val, true
}

func callbackCaptureReadOnly(closure *ssa.MakeClosure, cell ssa.Value, budget *SearchBudget) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, pair := range ClosureBindingPairs(function, closure) {
		if !budget.Spend() {
			return false
		}
		if pair.Binding != cell || pair.Free.Referrers() == nil {
			continue
		}
		for _, access := range *pair.Free.Referrers() {
			if !budget.Spend() {
				return false
			}
			load, ok := access.(*ssa.UnOp)
			if !ok || load.Op != token.MUL {
				return false
			}
		}
	}
	return true
}
