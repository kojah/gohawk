package heapmodel

import (
	"go/types"
	"strconv"
)

// Aggregate results can preserve fields that the callee never reads. Their
// snapshots resolve those fields lazily, so exporting only materialized slots
// loses the copy relationship. Projection asks the existing content model for
// bounded by-value struct fields; it does not follow pointers into other objects.
// https://github.com/urunc-dev/urunc/blob/ef1dc96a6bf0c188fc7714200d66d95557ae8af3/internal/metrics/metrics.go#L69-L73

func (projection *heapProjection) resultContents(state *regionState, object slot, typ types.Type, root HeapRoot) map[slot]pointees {
	if !isAggregate(typ) || object.path != "" || object.region.kind != regionSnapshot {
		return state.contents
	}
	contents := map[slot]pointees{}
	for target, set := range state.contents {
		if target.region == object.region {
			contents[target] = set
		}
	}
	count := 0
	var fields func(types.Type, string, int)
	fields = func(typ types.Type, path string, depth int) {
		if !tracked(typ) {
			return
		}
		if depth > SummaryPaths || count >= SummarySlots {
			projection.truncate(HeapSlot{Root: root})
			return
		}
		switch typed := typ.Underlying().(type) {
		case *types.Struct:
			for index := range typed.NumFields() {
				fields(typed.Field(index).Type(), joinSlotPath(path, "field:"+strconv.Itoa(index)), depth+1)
			}
		case *types.Array:
			// Array elements need their own bounded projection policy. A cut
			// prevents a missing element from becoming evidence of absence.
			projection.truncate(HeapSlot{Root: root, Path: path})
		default:
			count++
			target := slot{region: object.region, path: path}
			if _, materialized := contents[target]; !materialized {
				contents[target] = projection.copyContent(state, target)
			}
		}
	}
	fields(typ, "", 0)
	return contents
}

func (projection *heapProjection) copyContent(state *regionState, target slot) pointees {
	set := projection.graph.content(state, target)
	for pointee := range set {
		if pointee.region.kind == regionPlaceholder && pointee.region.stamp != (versionStamp{}) {
			// A placeholder after an opaque write names a later version of the
			// slot, while summaries name the caller's entry values. They cannot
			// express that version, so it must not become a preserved field.
			return pointees{{region: projection.graph.unkR}: false}
		}
	}
	return set
}
