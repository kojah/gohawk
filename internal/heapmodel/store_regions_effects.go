package heapmodel

import "github.com/kojah/gohawk/internal/ssaflow"

import (
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// Effects the graph cannot follow: calls, deferred calls, builtins, and map
// updates. Each rule errs toward forgetting, never toward inventing
// content, and escape is one-way.

// deferCall records what a deferred call was handed. Its arguments are
// evaluated now and its effects happen at RunDefers, so the objects are
// remembered rather than clobbered here; a load between the defer and the
// return still sees what the function stored.
func (graph *regionGraph) deferCall(state *regionState, deferred *ssa.Defer) {
	common := deferred.Common()
	arguments := append([]ssa.Value(nil), common.Args...)
	if common.IsInvoke() {
		arguments = append(arguments, common.Value)
	}
	_, closure := ssaflow.DirectCallee(common)
	if closure != nil {
		arguments = append(arguments, closure.Bindings...)
	} else {
		arguments = append(arguments, common.Value)
	}
	for _, argument := range arguments {
		state.deferred.union(graph.pointees(argument))
	}
	state.calls = append(state.calls, deferred)
}

// runDefers applies the deferred calls where they run. A deferred call
// with a summary is applied like any other call; one without forgets what
// it was handed, as an unresolved call would.
func (graph *regionGraph) runDefers(state *regionState, run *ssa.RunDefers) {
	unresolved := false
	for _, deferred := range state.calls {
		if _, builtin := deferred.Common().Value.(*ssa.Builtin); builtin {
			continue
		}
		if !graph.applyHeapSummary(state, deferred.Common(), deferred) {
			unresolved = true
		}
	}
	if !unresolved {
		return
	}
	graph.escape(state, state.deferred, HeapEscapedCall, run)
	graph.clobber(state, state.deferred, graph.id(run))
	graph.invalidateForeign(state, "", graph.id(run), reachEscaped)
}

// foreignReach says which objects an effect the graph cannot follow may
// have written.
type foreignReach uint8

const (
	// reachEscaped: an unresolved call. It can write what it was handed,
	// what has escaped before it, globals, and objects other code created;
	// under the structural contract it cannot reach a parameter or a local
	// that this function's own flow never let out.
	reachEscaped foreignReach = iota
	// reachAny: a store through a pointer the graph knows nothing about,
	// which may address anything.
	reachAny
)

// invalidateForeign forgets the contents of every object unknown code may
// have written, because a write through one such object may have reached
// any of them; the slot written itself is set right after by its caller.
// A step names the field or element written, so only slots ending in that
// step are forgotten and only their placeholders are restamped; an empty
// step forgets them all. The written slot's own exact entry, set
// afterwards, overrides the forgetting.
func (graph *regionGraph) invalidateForeign(state *regionState, step string, stamp int, reach foreignReach) {
	for target := range state.contents {
		if !graph.foreign(state, target.region, reach) {
			continue
		}
		if step == "" || stepKey(target.path) == step || isIndexStep(step) && isIndexStep(lastStep(target.path)) {
			delete(state.contents, target)
			if target.region.kind == regionSite {
				state.clobbered[target] = stamp
			}
		}
	}
	for target := range state.backing {
		if graph.foreign(state, target.region, reach) && (step == "" || target.region.kind == regionSite) {
			delete(state.backing, target)
			state.clobbered[target] = stamp
		}
	}
	// An escaped local the function never wrote has no entry to forget,
	// and would still read as its zero value: a variable captured by a
	// callback and assigned only inside it read as nil after the call that
	// ran the callback. Every escaped site is marked clobbered instead,
	// whole even when one step was written, because its unwritten slots
	// cannot be listed; the slots it did write keep their own entries.
	for object := range state.escaped {
		if object.kind == regionSite {
			state.clobbered[slot{region: object}] = stamp
		}
	}
	if step == "" {
		if reach == reachAny {
			state.epoch = stamp
		}
		state.reachEpoch = stamp
		return
	}
	state.stepEpochs[step] = stamp
	if isIndexStep(step) {
		for other := range state.stepEpochs {
			if isIndexStep(other) {
				state.stepEpochs[other] = stamp
			}
		}
		state.stepEpochs[pathStar] = stamp
	}
}

// unknownReach is the set of escapes that put an object within reach of
// code the graph did not follow: handed to a call, started, stored in a
// global, or sent. A store into a field is not one of them by itself; it
// is one only when the containing object was already within reach, and
// the store then propagates that reach to what it stored.
const unknownReach = HeapEscapedGlobal | HeapEscapedCall | HeapEscapedAsync | HeapEscapedSend

// foreign reports whether an object may be written by code the graph did
// not follow. A global, and an object some callee created, always may. A
// site, a parameter, a captured variable, and the content read out of any
// of them may only once this function let them out. A store through an
// unknown pointer may reach anything the function did not allocate.
func (graph *regionGraph) foreign(state *regionState, object *region, reach foreignReach) bool {
	switch object.kind {
	case regionSite:
		return state.escaped[object]
	case regionExternal:
		if _, global := object.origin.(*ssa.Global); global || reach == reachAny {
			return true
		}
		return state.escapes[slot{region: object}]&unknownReach != 0
	case regionOpaque:
		return true
	case regionPlaceholder:
		return reach == reachAny || state.escapes[slot{region: object}]&unknownReach != 0 || graph.foreign(state, object.source.region, reach)
	case regionClosure:
		return reach == reachAny || state.escapes[slot{region: object}]&unknownReach != 0
	case regionNil, regionUnknown, regionSnapshot:
		return false
	}
	return false
}

// reachOf is the reach a value stored into the object inherits: a global's
// and a callee-created object's, and whatever unknown code could already
// reach the object by.
func (graph *regionGraph) reachOf(state *regionState, object *region) HeapEscape {
	switch object.kind {
	case regionExternal:
		if _, global := object.origin.(*ssa.Global); global {
			return HeapEscapedGlobal
		}
	case regionOpaque:
		return HeapEscapedCall
	case regionSite, regionPlaceholder, regionClosure, regionSnapshot, regionNil, regionUnknown:
	}
	return state.escapes[slot{region: object}] & unknownReach
}

// escape marks every slot reachable from the pointees as having left local
// control in the given way: a site is then clobbered by any later effect,
// and the heap projection reports the escape for every slot the caller can
// name. What a slot holds escapes with it, and so does everything beneath
// it, but not the object above it: the address of a field hands on the
// field, not its container.
func (graph *regionGraph) escape(state *regionState, set pointees, kind HeapEscape, at ssa.Instruction) {
	// Each slot is queued once, when first reached, so the walk does the
	// work of one pass over the contents per slot it escapes.
	seen := map[slot]bool{}
	queue := make([]slot, 0, len(set))
	push := func(target slot) {
		if !seen[target] && target.region.kind != regionNil && target.region.kind != regionUnknown {
			seen[target] = true
			queue = append(queue, target)
		}
	}
	for target := range set {
		push(target)
	}
	for len(queue) > 0 {
		target := queue[0]
		queue = queue[1:]
		if state.escapes[target]&kind != kind {
			graph.recordEscape(target, kind, at)
		}
		state.escapes[target] |= kind
		if target.region.kind == regionSite {
			state.escaped[target.region] = true
		}
		for held, contents := range state.contents {
			if held.region == target.region && slotBeneath(held.path, target.path) {
				for pointee := range contents {
					push(pointee)
				}
			}
		}
	}
}

// recordEscape keeps, for the dump, the first instruction that escaped
// the slot in each way, so an escape effect in a summary can be traced to
// the instruction that produced it.
func (graph *regionGraph) recordEscape(target slot, kind HeapEscape, at ssa.Instruction) {
	if graph.escapeOrigins == nil {
		graph.escapeOrigins = map[escapeOrigin]ssa.Instruction{}
	}
	for _, single := range []HeapEscape{HeapEscapedCall, HeapEscapedAsync, HeapEscapedGlobal, HeapEscapedField, HeapEscapedSend} {
		key := escapeOrigin{target: target, kind: single}
		if kind&single == 0 {
			continue
		}
		if _, ok := graph.escapeOrigins[key]; !ok {
			graph.escapeOrigins[key] = at
		}
	}
}

// escapeOrigin keys the first instruction that escaped an object one way.
type escapeOrigin struct {
	target slot
	kind   HeapEscape
}

// clobber forgets the contents of every site reachable from the pointees,
// stamping them with the effect that did it. A closure handed on can only
// touch a captured cell through its own body, so a cell that body only
// reads keeps its contents whatever the callee does with the closure.
func (graph *regionGraph) clobber(state *regionState, set pointees, stamp int) {
	queue := make([]*region, 0, len(set))
	for target := range set {
		queue = append(queue, target.region)
	}
	seen := map[*region]bool{}
	for len(queue) > 0 {
		object := queue[0]
		queue = queue[1:]
		if seen[object] {
			continue
		}
		seen[object] = true
		for target, contents := range state.contents {
			if target.region != object {
				continue
			}
			for pointee := range contents {
				if object.kind == regionClosure && graph.closureOnlyReads(object, pointee) {
					continue
				}
				queue = append(queue, pointee.region)
			}
		}
		if object.kind == regionSite {
			state.clobbered[slot{region: object}] = stamp
			for target := range state.contents {
				if target.region == object {
					delete(state.contents, target)
				}
			}
			for target := range state.backing {
				if target.region == object {
					delete(state.backing, target)
				}
			}
		}
	}
}

// closureOnlyReads reports whether the captured cell is only read by the
// closure's body.
func (graph *regionGraph) closureOnlyReads(closure *region, captured slot) bool {
	literal, ok := closure.origin.(*ssa.MakeClosure)
	cell, isCell := captured.region.origin.(*ssa.Alloc)
	return ok && isCell && captured.region.kind == regionSite && captured.path == "" && ssaflow.CallbackCaptureReadOnly(literal, cell, graph.budget)
}

// call applies a call's effects. Results are opaque objects. Every object
// the function did not allocate may be written by any callee, and an
// escaped site with it. An unescaped site whose address reaches the callee
// keeps its contents only when the local effect proof shows the callee
// merely reads it; otherwise it is clobbered, and retained or started work
// escapes it. A callee the graph cannot resolve escapes everything it is
// handed.
func (graph *regionGraph) call(state *regionState, common *ssa.CallCommon, instruction ssa.Instruction, started bool) {
	if value, ok := instruction.(ssa.Value); ok {
		graph.setValue(value, pointees{{region: graph.opaque(value)}: false})
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		graph.builtin(state, builtin, common, instruction)
		return
	}
	if started {
		graph.recordCall(appliedSummary{instruction: instruction, callee: common.StaticCallee(), reason: CallStarted})
	} else if graph.applyHeapSummary(state, common, instruction) {
		return
	}
	arguments := append([]ssa.Value(nil), common.Args...)
	kind := HeapEscapedCall
	if started {
		kind = HeapEscapedAsync
	}
	if common.IsInvoke() {
		arguments = append(arguments, common.Value)
	}
	// A function value that is invoked is not handed anywhere: only what
	// the call receives can be kept by it.
	callee, closure := ssaflow.DirectCallee(common)
	if closure != nil {
		arguments = append(arguments, closure.Bindings...)
	}
	if callee == nil || len(callee.Blocks) == 0 || started {
		graph.unresolvedCall(state, arguments, kind, instruction)
		graph.runCallback(state, common, instruction)
		return
	}
	graph.invalidateForeign(state, "", graph.id(instruction), reachEscaped)
	effects := ssaflow.NewCallEffects(graph.budget)
	for _, argument := range arguments {
		set := graph.pointees(argument)
		if closure != nil && slices.Contains(closure.Bindings, argument) {
			if !ssaflow.CallbackCaptureReadOnly(closure, argument, graph.budget) {
				graph.escape(state, set, kind, instruction)
				graph.clobber(state, set, graph.id(instruction))
			}
			continue
		}
		proof := effects.Call(instruction, argument)
		switch {
		case proof.PreservesStorage():
		case proof.Proven() && proof.Effects&(ssaflow.EffectRetain|ssaflow.EffectAsync) == 0:
			graph.clobber(state, set, graph.id(instruction))
		default:
			graph.escape(state, set, kind, instruction)
			graph.clobber(state, set, graph.id(instruction))
		}
	}
}

// runCallback records a function value the graph cannot see being run: a
// callback parameter invoked by a helper writes through whatever it
// captured. Running it is not keeping it, so nothing escapes; what it
// reaches is clobbered, and the summary cuts the root it ran, so a caller
// forgets the captured variables of the closure it passed.
func (graph *regionGraph) runCallback(state *regionState, common *ssa.CallCommon, instruction ssa.Instruction) {
	if common.IsInvoke() || common.StaticCallee() != nil {
		return
	}
	set := graph.pointees(common.Value)
	for target := range set {
		state.ran[target.region] = true
	}
	graph.clobber(state, set, graph.id(instruction))
}

// unresolvedCall applies a call the graph cannot follow. What the call is
// handed escapes first, so the forgetting reaches it and everything it
// holds and nothing the function never let out, and is then clobbered.
func (graph *regionGraph) unresolvedCall(state *regionState, arguments []ssa.Value, kind HeapEscape, instruction ssa.Instruction) {
	state.opaque = true
	for _, argument := range arguments {
		graph.escape(state, graph.pointees(argument), kind, instruction)
	}
	graph.invalidateForeign(state, "", graph.id(instruction), reachEscaped)
	for _, argument := range arguments {
		graph.clobber(state, graph.pointees(argument), graph.id(instruction))
	}
}

// builtin applies the builtins that touch memory. Append may share the
// backing array with its first argument and stores the rest into it; copy
// stores the source's elements into the destination.
func (graph *regionGraph) builtin(state *regionState, builtin *ssa.Builtin, common *ssa.CallCommon, instruction ssa.Instruction) {
	switch builtin.Name() {
	case "append":
		value, _ := instruction.(ssa.Value)
		if len(common.Args) == 0 || value == nil {
			return
		}
		result := graph.pointees(common.Args[0]).clone()
		result.add(slot{region: graph.opaque(value)}, false)
		graph.setValue(value, result)
		for _, argument := range common.Args[1:] {
			elements := graph.pointees(argument)
			if _, ok := argument.Type().Underlying().(*types.Slice); ok {
				// The spread slice's elements are copied into a collection
				// the function may hand on, so the array behind it has
				// escaped as far as its contents are concerned.
				graph.escape(state, elements, HeapEscapedField, instruction)
				elements = graph.load(state, graph.selectStep(elements, pathStar), argument)
			}
			for target := range result {
				graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, pathStar)}, elements)
			}
			graph.escape(state, elements, HeapEscapedField, instruction)
		}
	case "copy":
		if len(common.Args) < 2 {
			return
		}
		elements := graph.load(state, graph.selectStep(graph.pointees(common.Args[1]), pathStar), common.Args[1])
		for target := range graph.pointees(common.Args[0]) {
			graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, pathStar)}, elements)
		}
	case "delete", "len", "cap", "print", "println", "close", "recover", "min", "max", "clear", "panic", "real", "imag", "complex", "new":
	}
}

// mapUpdate stores the value at the map's element slot and the key at its
// key slot. A key is kept as surely as a value: a range over the map hands
// it back, so an object used as a key has been stored, not merely compared.
func (graph *regionGraph) mapUpdate(state *regionState, update *ssa.MapUpdate) {
	value := graph.pointees(update.Value)
	key := graph.pointees(update.Key)
	graph.escape(state, value, HeapEscapedField, update)
	graph.escape(state, key, HeapEscapedField, update)
	for target := range graph.pointees(update.Map) {
		graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, "elem")}, value)
		graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, "key")}, key)
	}
}

func (graph *regionGraph) lookup(state *regionState, lookup *ssa.Lookup) {
	result := pointees{{region: graph.opaque(lookup)}: false}
	if _, ok := lookup.X.Type().Underlying().(*types.Map); ok {
		for target := range graph.pointees(lookup.X) {
			if set, ok := state.contents[slot{region: target.region, path: joinSlotPath(target.path, "elem")}]; ok {
				result.union(set)
			}
		}
	}
	graph.setValue(lookup, result)
}
