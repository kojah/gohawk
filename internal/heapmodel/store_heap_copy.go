package heapmodel

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// A by-value result is a new aggregate even when different exits return the
// entry value or a modified copy. Join its bounded reference fields per exit,
// rather than discarding them because the snapshots have different identities.
// Pointer results keep their original identity policy in projectResults.
// https://github.com/rs/zerolog/blob/116c8060e034e8d46855354d22db2acbc8df9e1e/log.go#L328-L337
func (projection *heapProjection) projectConditionalCopy(
	state *regionState, result ssa.Value, root HeapRoot, set pointees, record func(HeapSlot, pointees),
) bool {
	if !isAggregate(result.Type()) || len(projection.results[root.Index]) <= 1 {
		return false
	}
	if projection.copies == nil {
		projection.copies = map[int]*region{}
	}
	object := projection.copies[root.Index]
	if object == nil {
		object = &region{kind: regionOpaque}
		projection.copies[root.Index] = object
	}
	record(HeapSlot{Root: root}, pointees{{region: object}: false})
	walkStructReferences(result.Type(), func(path string) {
		contents := pointees{}
		for source, stale := range set {
			target := slot{region: source.region, path: joinSlotPath(source.path, path)}
			for pointee, old := range projection.copyContent(state, target) {
				contents.add(pointee, stale || old)
			}
		}
		if len(contents) == 0 {
			contents.add(slot{region: projection.graph.unkR}, false)
		}
		record(HeapSlot{Root: root, Path: path}, contents)
	}, func(path string) { projection.truncate(HeapSlot{Root: root, Path: path}) })
	return true
}

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
	walkStructReferences(typ, func(path string) {
		target := slot{region: object.region, path: path}
		if _, materialized := contents[target]; !materialized {
			contents[target] = projection.copyContent(state, target)
		}
	}, func(path string) { projection.truncate(HeapSlot{Root: root, Path: path}) })
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
