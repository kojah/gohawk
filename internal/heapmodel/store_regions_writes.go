package heapmodel

import (
	"maps"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// Definite and possible stores share destination selection, exposure and
// aggregate/content updates. Only a definite store into one exact destination
// replaces old contents; every conditional update retains the old possibilities.
func (graph *regionGraph) store(state *regionState, stored *ssa.Store) {
	graph.storeInto(state, stored.Addr, stored.Val, stored)
}

// storeInto writes written through address, as the instruction at does.
func (graph *regionGraph) storeInto(state *regionState, address, written ssa.Value, at ssa.Instruction) {
	graph.storeValue(state, address, written, at, true)
}

// A conditional update can retain the old contents even at one exact address.
// Reuse the same selection, exposure and union mechanics as uncertain SSA stores.
func (graph *regionGraph) storeConditionallyInto(state *regionState, address, written ssa.Value, at ssa.Instruction) {
	graph.storeValue(state, address, written, at, false)
}

func (graph *regionGraph) storeValue(state *regionState, address, written ssa.Value, at ssa.Instruction, definite bool) {
	targets := graph.pointees(address)
	value := graph.pointees(written)
	strong := definite && len(targets) == 1
	for target, stale := range targets {
		if stale || target.region.kind == regionUnknown || lastStep(target.path) == pathStar {
			strong = false
		}
	}
	if targets.unknown() {
		state.opaque = true
		graph.invalidateForeign(state, "", graph.id(at), reachAny)
		graph.escape(state, value, HeapEscapedField, at)
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
			graph.escape(state, value, escapeInto(target.region)|graph.reachOf(state, target.region), at)
		}
		if target.region.kind != regionSite {
			steps[stepKey(target.path)] = true
		}
	}
	for _, step := range slices.Sorted(maps.Keys(steps)) {
		graph.invalidateForeign(state, step, graph.id(at), reachAny)
	}
	for target := range targets {
		if target.region.kind == regionNil {
			continue
		}
		if isAggregate(written.Type()) {
			graph.storeAggregate(state, target, value, strong, graph.id(at))
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
		graph.bound(state, target, at)
	}
}
