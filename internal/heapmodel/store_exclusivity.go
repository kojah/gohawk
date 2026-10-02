package heapmodel

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Whole-object exclusivity permits an imprecise selection within one non-stale
// object. It requires a complete identity and no recorded exposure at the
// observation; exact content identity remains the separate slot query policy.

// ExclusiveObject says who can reach an object at an instruction: only the
// function, through a local allocation that has not escaped there, or only
// the function and its caller, through a parameter that has not escaped
// there. Published reports whether the local object escapes later on some
// path, which separates an object being initialized before publication
// from one that never leaves the function.
type ExclusiveObject struct {
	Parameter int
	Local     bool
	Published bool
}

// exclusiveAt reports whether the one object the value refers into has not
// escaped when the instruction runs, and who else could reach it.
func (graph *regionGraph) exclusiveAt(value ssa.Value, at ssa.Instruction) (ExclusiveObject, bool) {
	defer graph.lock()()
	set, ok := graph.pointsToUnlocked(value)
	if !ok {
		return ExclusiveObject{}, false
	}
	target, ok := singleObjectSlot(set)
	if !ok {
		return ExclusiveObject{}, false
	}
	state := graph.stateAt(at)
	if state == nil || regionExposed(state, target.region) {
		return ExclusiveObject{}, false
	}
	switch target.region.kind {
	case regionSite:
		if state.escaped[target.region] {
			return ExclusiveObject{}, false
		}
		return ExclusiveObject{Local: true, Published: graph.publishedAfterUnlocked(target.region, at)}, true
	case regionExternal:
		parameter, ok := target.region.origin.(*ssa.Parameter)
		if !ok {
			return ExclusiveObject{}, false
		}
		for index, candidate := range graph.function.Params {
			if candidate == parameter {
				return ExclusiveObject{Parameter: index}, true
			}
		}
	case regionOpaque:
		// A language allocation is fresh even though its element content is
		// opaque. Any exposed selection defeats whole-object exclusivity.
		if freshMake(target.region.origin) {
			return ExclusiveObject{Local: true, Published: graph.publishedAfterUnlocked(target.region, at)}, true
		}
	case regionNil, regionUnknown, regionPlaceholder, regionSnapshot, regionClosure:
	}
	return ExclusiveObject{}, false
}

// freshMake separates language allocations from opaque calls and loaded values.
func freshMake(origin ssa.Value) bool {
	switch origin.(type) {
	case *ssa.MakeMap, *ssa.MakeSlice, *ssa.MakeChan:
		return true
	}
	return false
}

func regionExposed(state *regionState, object *region) bool {
	for target, escape := range state.escapes {
		if target.region == object && escape != 0 {
			return true
		}
	}
	return false
}

// publishedAfter reports whether the object is published on some path
// from the instruction: a return the instruction can reach whose state has
// the object stored into a global or a field, sent, or handed to a
// goroutine. Being handed to a call is not publication. Escapes accumulate
// along a path, so a publication seen at such a return and absent at the
// instruction happened between them.
func (graph *regionGraph) publishedAfterUnlocked(object *region, at ssa.Instruction) bool {
	const publication = HeapEscapedGlobal | HeapEscapedField | HeapEscapedSend | HeapEscapedAsync
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](graph.function) {
		if !ssaflow.InstructionMayFollow(at, returned) {
			continue
		}
		if state := graph.stateAt(returned); state != nil && state.escapes[slot{region: object}]&publication != 0 {
			return true
		}
	}
	return false
}

// singleObjectSlot permits different selections of one object, retaining the
// stale and unknown-object boundaries. A wildcard element still belongs to that
// object; exact storage identity continues to require singleSlot.
func singleObjectSlot(set pointees) (slot, bool) {
	var object slot
	for target, stale := range set {
		if stale || target.region.kind == regionUnknown || object.region != nil && object.region != target.region {
			return slot{}, false
		}
		object = slot{region: target.region}
	}
	return object, object.region != nil
}
