package ssainfer

import (
	"go/constant"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
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

// invokesLocal reports whether a call's function value is the local itself or
// a load of it; a parameter captured by a literal is spilled to a cell and
// invoked through a load.
func invokesLocal(value, local ssa.Value) bool {
	if DefinitelySameValue(value, local) {
		return true
	}
	load, ok := value.(*ssa.UnOp)
	return ok && load.Op == token.MUL && load.X == local
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
	for _, binding := range ssaflow.CallBindings(callee.common, callee.function, callee.closure) {
		if !search.budget.Spend() {
			break
		}
		environment := search.bindings
		if binding.Captured {
			environment = callee.environment
		}
		bindings.values[binding.Local] = callbackValue{binding.Supplied, environment, callee.invocation}
	}
	return bindings
}

func (search *completionSearch) boundCallees(instruction ssa.Instruction) ([]completionCallee, bool) {
	common := ssaflow.InstructionCall(instruction)
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

// Resolve only immutable values or a cell with proven stable contents.
// Capturing a cell does not make its contents immutable: reject reassignment
// and opaque address escape rather than using the value at registration time.
func resolveCallbackValue(ref callbackValue, budget *ssaflow.SearchBudget) (callbackValue, bool) {
	walk := ssaflow.NewReachingWalk(ssaflow.TransparentChangeType | ssaflow.TransparentConvert)
	return resolveCallbackBinding(walk, ref, budget)
}

func resolveCallbackBinding(walk ssaflow.ReachingWalk, ref callbackValue, budget *ssaflow.SearchBudget) (callbackValue, bool) {
	if ref.value == nil || !budget.Spend() || !walk.Mark(ref.value) {
		return callbackValue{}, false
	}
	if ref.bindings != nil {
		if next, ok := ref.bindings.values[ref.value]; ok {
			return resolveCallbackBinding(walk, next, budget)
		}
	}
	if inner, ok := ssaflow.UnwrapTransparentValue(ref.value, ssaflow.TransparentChangeType|ssaflow.TransparentConvert); ok {
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
		if stored, ok := NewStorage(budget).stableValue(value, ref.observation); ok {
			ref.value = stored
			return resolveCallbackBinding(walk, ref, budget)
		}
	}
	return callbackValue{}, false
}

// A projection is resolved only through an unchanged local aggregate. Unknown
// calls receiving its address can mutate a field without an SSA Store here.
func callbackAggregate(ref callbackValue, budget *ssaflow.SearchBudget) (callbackValue, bool) {
	for budget.Spend() {
		if ref.bindings != nil {
			if next, ok := ref.bindings.values[ref.value]; ok {
				if !ssaflow.NewCallEffects(budget).Value(ref.value).PreservesStorage() {
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

func resolveCallbackField(walk ssaflow.ReachingWalk, ref callbackValue, field *ssa.FieldAddr, budget *ssaflow.SearchBudget) (callbackValue, bool) {
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
		value, ok := NewStorage(budget).stableValue(address, root.observation)
		if !ok || stored != nil && stored != value {
			return callbackValue{}, false
		}
		stored = value
	}
	root.value = stored
	return resolveCallbackBinding(walk, root, budget)
}

func resolveCallbackElement(walk ssaflow.ReachingWalk, ref callbackValue, index *ssa.IndexAddr, budget *ssaflow.SearchBudget) (callbackValue, bool) {
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
		value, ok := NewStorage(budget).stableValue(address, root.observation)
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
