package ssaflow

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
	_, closure := DirectCallee(common)
	if closure != nil {
		arguments = append(arguments, closure.Bindings...)
	} else {
		arguments = append(arguments, common.Value)
	}
	for _, argument := range arguments {
		state.deferred.union(graph.pointees(argument))
	}
}

// invalidateForeign forgets the contents of every object the function did
// not allocate, and of every escaped site, because a write through one such
// object may have reached any of them; the slot written itself is set right
// after by its caller. A step names the field or element written, so only
// slots ending in that step are forgotten and only their placeholders are
// restamped; an empty step forgets them all. The written slot's own exact
// entry, set afterwards, overrides the forgetting.
func (graph *regionGraph) invalidateForeign(state *regionState, step string, stamp int) {
	for target := range state.contents {
		if !graph.foreign(state, target.region) {
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
		if graph.foreign(state, target.region) && (step == "" || target.region.kind == regionSite) {
			delete(state.backing, target)
			state.clobbered[target] = stamp
		}
	}
	if step == "" {
		state.epoch = stamp
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

// foreign reports whether an object may be written through a pointer the
// function did not derive from a local allocation.
func (graph *regionGraph) foreign(state *regionState, object *region) bool {
	switch object.kind {
	case regionSite:
		return state.escaped[object]
	case regionExternal, regionOpaque, regionPlaceholder:
		return true
	case regionNil, regionUnknown, regionSnapshot, regionClosure:
		return false
	}
	return false
}

// escape marks every site reachable from the pointees as escaped: its
// address is now held somewhere the function cannot see.
func (graph *regionGraph) escape(state *regionState, set pointees) {
	queue := make([]*region, 0, len(set))
	for target := range set {
		queue = append(queue, target.region)
	}
	for len(queue) > 0 {
		object := queue[0]
		queue = queue[1:]
		if object.kind == regionSnapshot || object.kind == regionClosure {
			for target, contents := range state.contents {
				if target.region == object {
					for pointee := range contents {
						queue = append(queue, pointee.region)
					}
				}
			}
			continue
		}
		if object.kind != regionSite || state.escaped[object] {
			continue
		}
		state.escaped[object] = true
		for target, contents := range state.contents {
			if target.region == object {
				for pointee := range contents {
					queue = append(queue, pointee.region)
				}
			}
		}
	}
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
	return ok && isCell && captured.region.kind == regionSite && captured.path == "" &&
		callbackCaptureReadOnly(literal, cell, graph.budget)
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
	graph.invalidateForeign(state, "", graph.id(instruction))
	arguments := append([]ssa.Value(nil), common.Args...)
	if common.IsInvoke() {
		arguments = append(arguments, common.Value)
	}
	callee, closure := DirectCallee(common)
	if closure != nil {
		arguments = append(arguments, closure.Bindings...)
	} else if callee == nil {
		arguments = append(arguments, common.Value)
	}
	effects := NewCallEffects(graph.budget)
	for _, argument := range arguments {
		set := graph.pointees(argument)
		if callee == nil || len(callee.Blocks) == 0 || started {
			graph.escape(state, set)
			graph.clobber(state, set, graph.id(instruction))
			continue
		}
		if closure != nil && slices.Contains(closure.Bindings, argument) {
			if !callbackCaptureReadOnly(closure, argument, graph.budget) {
				graph.escape(state, set)
				graph.clobber(state, set, graph.id(instruction))
			}
			continue
		}
		proof := effects.Call(instruction, argument)
		switch {
		case proof.PreservesStorage():
		case proof.Proven() && proof.Effects&(EffectRetain|EffectAsync) == 0:
			graph.clobber(state, set, graph.id(instruction))
		default:
			graph.escape(state, set)
			graph.clobber(state, set, graph.id(instruction))
		}
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
				graph.escape(state, elements)
				elements = graph.load(state, graph.selectStep(elements, pathStar), argument)
			}
			for target := range result {
				graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, pathStar)}, elements)
			}
			graph.escape(state, elements)
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

func (graph *regionGraph) mapUpdate(state *regionState, update *ssa.MapUpdate) {
	value := graph.pointees(update.Value)
	graph.escape(state, value)
	for target := range graph.pointees(update.Map) {
		graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, "elem")}, value)
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
