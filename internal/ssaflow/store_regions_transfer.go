package ssaflow

import (
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

// The transfer function moves the memory state across one instruction and
// records the pointees of the value it defines. It is the one place the
// graph decides what each SSA form does to memory; the queries only read
// what it recorded.

// transfer applies one instruction to the state: the instructions that
// touch memory are handled here, and every other instruction only defines a
// value whose pointees define records.
func (graph *regionGraph) transfer(state *regionState, instruction ssa.Instruction) {
	switch typed := instruction.(type) {
	case *ssa.Alloc:
		graph.allocate(state, typed)
	case *ssa.Store:
		graph.store(state, typed)
	case *ssa.Call:
		graph.call(state, typed.Common(), typed, false)
	case *ssa.Defer:
		graph.deferCall(state, typed)
	case *ssa.RunDefers:
		graph.runDefers(state, typed)
	case *ssa.Go:
		graph.call(state, typed.Common(), typed, true)
	case *ssa.Send:
		graph.escape(state, graph.pointees(typed.X), HeapEscapedSend, typed)
	case *ssa.MapUpdate:
		graph.mapUpdate(state, typed)
	case *ssa.Lookup:
		graph.lookup(state, typed)
	case *ssa.MakeClosure:
		graph.makeClosure(state, typed)
	case *ssa.Phi:
		graph.phi(typed)
	case *ssa.Extract:
		graph.extract(typed)
	default:
		graph.define(state, instruction)
	}
}

// define records the pointees of a value-defining instruction that does not
// change memory.
func (graph *regionGraph) define(state *regionState, instruction ssa.Instruction) {
	if operand, ok := wrapperOperand(instruction); ok {
		graph.setValue(instruction.(ssa.Value), graph.pointees(operand)) //nolint:forcetypeassert // A wrapper is a value.
		return
	}
	switch typed := instruction.(type) {
	case *ssa.FieldAddr:
		graph.setValue(typed, graph.selectStep(graph.pointees(typed.X), "field:"+strconv.Itoa(typed.Field)))
	case *ssa.Field:
		graph.setValue(typed, graph.load(state, graph.selectStep(graph.pointees(typed.X), "field:"+strconv.Itoa(typed.Field)), typed))
	case *ssa.IndexAddr:
		graph.setValue(typed, graph.index(typed.X, typed.Index))
	case *ssa.Index:
		graph.setValue(typed, graph.load(state, graph.index(typed.X, typed.Index), typed))
	case *ssa.Slice:
		graph.slice(typed)
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			graph.setValue(typed, graph.load(state, graph.pointees(typed.X), typed))
		}
	case *ssa.TypeAssert:
		if !typed.CommaOk {
			graph.setValue(typed, graph.pointees(typed.X))
		}
	case *ssa.MakeMap, *ssa.MakeChan, *ssa.MakeSlice, *ssa.Range, *ssa.Next, *ssa.BinOp, *ssa.MultiConvert:
		if value, ok := typed.(ssa.Value); ok {
			graph.setValue(value, pointees{{region: graph.opaque(value)}: false})
		}
	case *ssa.Return, *ssa.Jump, *ssa.If, *ssa.Panic, *ssa.DebugRef, *ssa.Select:
	}
}

// wrapperOperand returns the operand of an instruction whose value refers
// to exactly what its operand refers to.
func wrapperOperand(instruction ssa.Instruction) (ssa.Value, bool) { //nolint:ireturn // Operands keep their concrete forms.
	switch typed := instruction.(type) {
	case *ssa.ChangeInterface:
		return typed.X, true
	case *ssa.ChangeType:
		return typed.X, true
	case *ssa.Convert:
		return typed.X, true
	case *ssa.MakeInterface:
		return typed.X, true
	case *ssa.SliceToArrayPointer:
		return typed.X, true
	}
	return nil, false
}

// pointees returns the slots a value may refer to, deriving them for the
// values no instruction defines: parameters, free variables, globals,
// constants, and functions.
func (graph *regionGraph) pointees(value ssa.Value) pointees {
	if value == nil {
		return pointees{{region: graph.unkR}: false}
	}
	if set, ok := graph.values[value]; ok {
		return set
	}
	if !tracked(value.Type()) {
		return pointees{}
	}
	if owner := valueFunction(value); owner != nil && owner != graph.function {
		// A value of another function, such as a closure's parameter or
		// free variable, may refer to anything this function holds.
		return pointees{{region: graph.unkR}: false}
	}
	var set pointees
	switch typed := value.(type) {
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Global:
		set = pointees{{region: graph.external(typed)}: false}
	case *ssa.Const:
		// A nil constant is nil; a non-nil constant of aggregate type is a
		// zero value, which holds nil in every slot.
		set = pointees{{region: graph.nilR}: false}
	case *ssa.Function, *ssa.Builtin:
		set = pointees{{region: graph.opaque(typed)}: false}
	case ssa.Instruction:
		// An instruction of this function the fixpoint has not reached yet,
		// which only a phi from a back edge can ask about, or an
		// instruction of another function.
		if typed.Parent() != graph.function {
			set = pointees{{region: graph.unkR}: false}
		} else {
			return pointees{}
		}
	default:
		set = pointees{{region: graph.unkR}: false}
	}
	graph.values[value] = set
	return set
}

// tracked reports whether values of the type can refer to or hold memory
// the graph models: a scalar cannot, and a function value is a closure
// object that can.
func tracked(typ types.Type) bool {
	switch typ.Underlying().(type) {
	case *types.Basic, *types.Tuple:
		return false
	}
	return true
}

// setValue records the pointees of a value, growing them across rounds.
func (graph *regionGraph) setValue(value ssa.Value, set pointees) {
	if !tracked(value.Type()) {
		return
	}
	existing, ok := graph.values[value]
	if !ok {
		graph.values[value] = set.clone()
		return
	}
	existing.union(set)
	graph.boundValue(value)
}

// allocate zeroes the site's storage. A site inside a loop is a fresh cell
// each iteration, but a pointer to the earlier cell may survive, so the
// zeroing is weak there.
func (graph *regionGraph) allocate(state *regionState, alloc *ssa.Alloc) {
	site := graph.site(alloc)
	graph.setValue(alloc, pointees{{region: site}: false})
	if BlockInCycle(alloc.Block()) && state.escaped[site] {
		return
	}
	for target := range state.contents {
		if target.region == site {
			delete(state.contents, target)
		}
	}
	for target := range state.backing {
		if target.region == site {
			delete(state.backing, target)
		}
	}
	for target := range state.clobbered {
		if target.region == site {
			delete(state.clobbered, target)
		}
	}
}

// selectStep returns the slots one step beneath each of the bases.
func (graph *regionGraph) selectStep(bases pointees, step string) pointees {
	result := pointees{}
	for base, stale := range bases {
		switch base.region.kind {
		case regionUnknown:
			result.add(base, stale)
		case regionNil:
			// Selecting through nil faults; the value never reaches a use.
			result.add(slot{region: graph.unkR}, stale)
		case regionSite, regionExternal, regionOpaque, regionPlaceholder, regionSnapshot, regionClosure:
			result.add(slot{region: base.region, path: joinSlotPath(base.path, step)}, stale)
		}
	}
	return result
}

// index returns the element slots selected from an array address, array
// value, or slice by the index, using the constant position when the
// window and index are both known and the element step otherwise.
func (graph *regionGraph) index(base, index ssa.Value) pointees {
	bases := graph.pointees(base)
	position, fixed := constantIndex(index)
	view, viewed := graph.views[base]
	if !viewed {
		if length, ok := arrayLength(base.Type()); ok {
			view, viewed = sliceView{size: length, capacity: length}, true
		}
	}
	step := pathStar
	if fixed && viewed {
		if offset, err := strconv.ParseInt(position, 10, 64); err == nil && offset >= 0 && offset < view.size {
			step = "index:" + strconv.FormatInt(view.offset+offset, 10)
		}
	}
	return graph.selectStep(bases, step)
}

// arrayLength returns the length of an array type or a pointer to one.
func arrayLength(typ types.Type) (int64, bool) {
	if pointer, ok := typ.Underlying().(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	array, ok := typ.Underlying().(*types.Array)
	if !ok {
		return 0, false
	}
	return array.Len(), true
}

// slice records a window over the sliced value's storage; a slice of a local
// array with constant bounds keeps naming the array's own elements.
func (graph *regionGraph) slice(sliced *ssa.Slice) {
	graph.setValue(sliced, graph.pointees(sliced.X))
	view, ok := graph.views[sliced.X]
	if !ok {
		length, isArray := arrayLength(sliced.X.Type())
		if !isArray {
			return
		}
		view = sliceView{size: length, capacity: length}
	}
	low, lowOK := storageInteger(sliced.Low, 0)
	high, highOK := storageInteger(sliced.High, view.size)
	max, maxOK := storageInteger(sliced.Max, view.capacity)
	if !lowOK || !highOK || !maxOK || low < 0 || low > high || high > max || max > view.capacity {
		return
	}
	graph.views[sliced] = sliceView{offset: view.offset + low, size: high - low, capacity: max - low}
}

// load returns what the slots hold. A whole-aggregate load makes a snapshot
// whose sub-slots are copied from the source.
func (graph *regionGraph) load(state *regionState, addresses pointees, loaded ssa.Value) pointees {
	if isAggregate(loaded.Type()) {
		// A slot written whole from an aggregate value holds that value: a
		// copy of a by-value parameter is the parameter. Otherwise the load
		// is a fresh copy of whatever the sub-slots hold.
		if address, ok := singleSlot(addresses); ok {
			if set, ok := state.contents[address]; ok && len(set) > 0 {
				return set.clone()
			}
		}
		return graph.snapshotOf(state, addresses, loaded)
	}
	result := pointees{}
	for address, stale := range addresses {
		for pointee, pointeeStale := range graph.content(state, address) {
			result.add(pointee, stale || pointeeStale)
			if pointee.region.kind == regionPlaceholder && pointee.region.origin == nil {
				// The first load that produced the placeholder names it.
				pointee.region.origin = loaded
			}
		}
	}
	return result
}

func isAggregate(typ types.Type) bool {
	switch typ.Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	}
	return false
}

// store writes the value into the addressed slots.
func (graph *regionGraph) store(state *regionState, stored *ssa.Store) {
	targets := graph.pointees(stored.Addr)
	value := graph.pointees(stored.Val)
	strong := len(targets) == 1
	for target, stale := range targets {
		if stale || target.region.kind == regionUnknown || lastStep(target.path) == pathStar {
			strong = false
		}
	}
	if targets.unknown() {
		state.opaque = true
		graph.invalidateForeign(state, "", graph.id(stored))
		graph.escape(state, value, HeapEscapedField, stored)
		return
	}
	// A write through a pointer that may reach an object the function did
	// not allocate may have written the same step of any such object, so
	// those slots are forgotten once, before any target is written: a
	// target written first must not be forgotten again for a target
	// written later, or the surviving entry would be whichever target the
	// walk visited last, and a build that visits them in another order
	// would never settle.
	steps := map[string]bool{}
	for target := range targets {
		if target.region.kind == regionNil {
			continue
		}
		if target.region.kind != regionSite || state.escaped[target.region] {
			graph.escape(state, value, escapeInto(target.region), stored)
		}
		if target.region.kind != regionSite {
			steps[stepKey(target.path)] = true
		}
	}
	for _, step := range slices.Sorted(maps.Keys(steps)) {
		graph.invalidateForeign(state, step, graph.id(stored))
	}
	for target := range targets {
		if target.region.kind == regionNil {
			continue
		}
		if isAggregate(stored.Val.Type()) {
			graph.storeAggregate(state, target, value, strong, graph.id(stored))
			continue
		}
		if lastStep(target.path) == pathStar {
			graph.weakElementStore(state, target, value)
			continue
		}
		graph.remember(target, value)
		if strong {
			graph.clearSubtree(state, target)
			graph.forgetWholeAbove(state, target)
			state.contents[target] = value.clone()
			continue
		}
		graph.forgetWholeAbove(state, target)
		existing, ok := state.contents[target]
		if !ok {
			existing = graph.content(state, target)
			state.contents[target] = existing
		}
		existing.union(value)
		graph.bound(state, target, stored)
	}
}

func (graph *regionGraph) phi(merged *ssa.Phi) {
	result := pointees{}
	block := merged.Block()
	for index, edge := range merged.Edges {
		predecessor := block.Preds[index]
		backEdge := block.Dominates(predecessor)
		for target, stale := range graph.pointees(edge) {
			result.add(target, stale || backEdge && createdInsideLoop(target.region, block))
		}
	}
	graph.setValue(merged, result)
}

func (graph *regionGraph) extract(extract *ssa.Extract) {
	if assertion, ok := extract.Tuple.(*ssa.TypeAssert); ok && extract.Index == 0 {
		graph.setValue(extract, graph.pointees(assertion.X))
		return
	}
	if call, ok := extract.Tuple.(*ssa.Call); ok {
		if results := graph.callResults[call]; extract.Index < len(results) && results[extract.Index] != nil {
			graph.setValue(extract, results[extract.Index])
			return
		}
	}
	graph.setValue(extract, pointees{{region: graph.opaque(extract)}: false})
}

func (graph *regionGraph) makeClosure(state *regionState, closure *ssa.MakeClosure) {
	object := graph.closure(closure)
	graph.setValue(closure, pointees{{region: object}: false})
	for index, binding := range closure.Bindings {
		target := slot{region: object, path: "binding:" + strconv.Itoa(index)}
		state.contents[target] = graph.pointees(binding).clone()
		graph.remember(target, state.contents[target])
	}
}
