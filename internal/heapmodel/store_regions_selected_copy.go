package heapmodel

import (
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Selected aggregate copies preserve the union of reference destinations in a
// known bounded array window. The snapshot remains clobbered outside those
// slots; an opaque window or element cannot establish a destination guarantee.
// https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/internal/admin/repository/sql_store.go#L73-L99
func (graph *regionGraph) selectedSnapshot(state *regionState, address slot, stale bool, loaded ssa.Value, snapshot *region) {
	load, ok := loaded.(*ssa.UnOp)
	if !ok {
		return
	}
	index, ok := load.X.(*ssa.IndexAddr)
	if !ok {
		return
	}
	view, known := graph.view(index.X)
	if !known || view.size <= 0 || view.size > SummarySlots {
		return
	}
	// A wildcard write beneath an element is not an exact element update.
	// Until the content model propagates those writes through all descendant
	// slots, preserve the clobbered snapshot instead of reusing old fields.
	for target := range state.contents {
		if target.region == address.region && strings.Contains(target.path, pathStar) {
			return
		}
	}
	for target := range state.clobbered {
		if target.region == address.region && strings.Contains(target.path, pathStar) {
			return
		}
	}
	walkStructReferences(loaded.Type(), func(path string) {
		contents := pointees{}
		for offset := range view.size {
			element := joinSlotPath(parentPath(address.path), "index:"+strconv.FormatInt(view.offset+offset, 10))
			source := slot{region: address.region, path: joinSlotPath(element, path)}
			for target, old := range graph.content(state, source) {
				contents.add(target, stale || old)
			}
		}
		target := slot{region: snapshot, path: path}
		state.contents[target] = contents
		graph.bound(state, target, load)
		graph.remember(target, state.contents[target])
	}, func(string) {})
}
