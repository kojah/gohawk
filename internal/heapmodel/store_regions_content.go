package heapmodel

import (
	"go/types"
	"maps"
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Slot content reads and writes share one versioned memory model. Aggregate
// copying and destination selection preserve definite versus possible contents;
// unknown selections never establish an exact occupant.

// Slot contents: what a location holds at a point, how an aggregate is
// copied whole, and how a store settles into the state. These rules decide
// the must-answers, so each one says where it stops being exact.

// content returns what one slot holds in the state: its own entry, a
// wildcard element entry beneath the same array, the backing snapshot's
// entry, nil for an unwritten local slot, or a placeholder for an unwritten
// slot of an object the function did not allocate. A clobbered prefix makes
// the answer unknown unless the slot itself was written since.
// Every query returns an independently owned, writable set.
func (graph *regionGraph) content(state *regionState, target slot) pointees {
	return graph.contentFollowing(state, target, nil)
}

// maxContentHops bounds nested copy resolution even when each hop names a
// different slot. Beyond it, the graph cannot claim exact contents.
const maxContentHops = 64

// contentFollowing keeps the visited slots local to this read. A backing
// copy or snapshot source may point back to an earlier slot; that cycle
// proves neither nil nor a particular stored object.
// https://github.com/golang/freetype/tree/e2365dfdc4a0/truetype
func (graph *regionGraph) contentFollowing(state *regionState, target slot, path []slot) pointees {
	if len(path) >= maxContentHops || slices.Contains(path, target) {
		return pointees{{region: graph.unkR}: false}
	}
	if target.region.kind == regionUnknown {
		return pointees{target: false}
	}
	if target.region.kind == regionNil {
		return pointees{{region: graph.nilR}: false}
	}
	// Unwritten reads return their own set. Allocate a merge result only
	// when stored evidence contributes or a dynamic read needs a union.
	var result pointees
	if set := state.contents[target]; len(set) > 0 {
		result = pointees{}
		result.union(set)
	}
	step := lastStep(target.path)
	if step == pathStar {
		if result == nil {
			result = pointees{}
		}
		for other, set := range state.contents {
			if other.region == target.region && parentPath(other.path) == parentPath(target.path) && isIndexStep(lastStep(other.path)) {
				result.union(set)
			}
		}
		// A dynamic read may also hit an element nobody wrote, which
		// the unwritten answer below describes.
		result.union(graph.unwritten(state, target, path))
		return result
	}
	if isIndexStep(step) {
		if set := state.contents[graph.starSlot(target)]; len(set) > 0 {
			if result == nil {
				result = pointees{}
			}
			result.union(set)
		}
	}
	if len(result) > 0 {
		return result
	}
	return graph.unwritten(state, target, path)
}

// unwritten returns what a slot the function did not write holds: after an
// effect the graph could not follow, an object stamped by that effect; the
// backing snapshot's content; nil for an untouched local; a placeholder for
// an object the function did not allocate.
func (graph *regionGraph) unwritten(state *regionState, target slot, path []slot) pointees {
	if stamp, ok := graph.clobberedBeneath(state, target); ok {
		return pointees{{region: graph.placeholder(target, versionStamp{epoch: stamp})}: false}
	}
	if backing, rest, ok := graph.backingOf(state, target); ok {
		return graph.contentFollowing(state, slot{region: backing, path: rest}, append(path, target))
	}
	switch target.region.kind {
	case regionSite:
		return pointees{{region: graph.nilR}: false}
	case regionSnapshot:
		source := target.region.source
		switch {
		case source.region == nil:
			return pointees{{region: graph.unkR}: false}
		case source.region.kind == regionSnapshot:
			return graph.contentFollowing(state, slot{region: source.region, path: joinSlotPath(source.path, target.path)}, append(path, target))
		case source.region.kind == regionSite:
			return pointees{{region: graph.nilR}: false}
		}
		origin := slot{region: source.region, path: joinSlotPath(source.path, target.path)}
		stamp := versionStamp{epoch: target.region.stamp.epoch, step: target.region.stamps[stepKey(origin.path)]}
		return pointees{{region: graph.placeholder(origin, stamp)}: false}
	case regionExternal, regionOpaque, regionPlaceholder, regionClosure:
		return pointees{{region: graph.placeholder(target, graph.stampOf(state, target))}: false}
	case regionNil, regionUnknown:
	}
	return pointees{{region: graph.unkR}: false}
}

func (graph *regionGraph) starSlot(target slot) slot {
	return slot{region: target.region, path: joinSlotPath(parentPath(target.path), pathStar)}
}

func parentPath(path string) string {
	if index := len(path) - len(lastStep(path)) - 1; index > 0 {
		return path[:index]
	}
	return ""
}

// clobberedBeneath returns the stamp of the clobber that reaches the slot:
// the most specific clobbered prefix above it, and the latest stamp among
// clobbers of that prefix, so the placeholder a read produces does not
// depend on the order the clobbers are visited in.
func (graph *regionGraph) clobberedBeneath(state *regionState, target slot) (int, bool) {
	best, found := slot{}, false
	stamp := 0
	for prefix, candidate := range state.clobbered {
		if prefix.region != target.region || !slotBeneath(target.path, prefix.path) {
			continue
		}
		closer := len(prefix.path) > len(best.path) || len(prefix.path) == len(best.path) && candidate > stamp
		if !found || closer {
			best, stamp, found = prefix, candidate, true
		}
	}
	return stamp, found
}

// backingOf finds the nearest slot above target that a snapshot was copied
// into, and the canonical relative path of target beneath it.
func (graph *regionGraph) backingOf(state *regionState, target slot) (*region, string, bool) {
	path := target.path
	for {
		if backing, ok := state.backing[slot{region: target.region, path: path}]; ok {
			// A nonempty prefix leaves a separating slash. The snapshot uses
			// relative paths; keeping that slash names a different source slot
			// and loses identity for an unchanged nested copy.
			return backing, trimSlash(target.path[len(path):]), true
		}
		if path == "" {
			return nil, "", false
		}
		path = parentPath(path)
	}
}

// snapshotOf copies an aggregate out of the addressed slots into a fresh
// snapshot region for the loaded value.
func (graph *regionGraph) snapshotOf(state *regionState, addresses pointees, loaded ssa.Value) pointees {
	snapshot := graph.snapshot(loaded)
	graph.clearSubtree(state, slot{region: snapshot})
	stamp := 0
	if instruction, ok := loaded.(ssa.Instruction); ok {
		stamp = graph.id(instruction)
	}
	if len(addresses) != 1 || addresses.unknown() {
		state.clobbered[slot{region: snapshot}] = stamp
		return pointees{{region: snapshot}: false}
	}
	for address, stale := range addresses {
		if lastStep(address.path) == pathStar {
			state.clobbered[slot{region: snapshot}] = stamp
			graph.selectedSnapshot(state, address, stale, loaded, snapshot)
			return pointees{{region: snapshot}: stale}
		}
		graph.copySubtree(state, address, slot{region: snapshot})
		snapshot.source = address
		// The copy reads its source's unwritten slots as the placeholders
		// a load there would produce now, stamped by the last effect that
		// could have reached the source.
		snapshot.stamp = versionStamp{epoch: graph.stampOf(state, address).epoch}
		snapshot.stamps = maps.Clone(state.stepEpochs)
		if clobber, ok := graph.clobberedBeneath(state, address); ok {
			state.clobbered[slot{region: snapshot}] = clobber
		}
		return pointees{{region: snapshot}: stale}
	}
	return pointees{{region: snapshot}: false}
}

// copySubtree copies every entry beneath source to the same path beneath
// destination and carries the backing snapshot of source along.
func (graph *regionGraph) copySubtree(state *regionState, source, destination slot) {
	for target, set := range state.contents {
		if target.region != source.region || !slotBeneath(target.path, source.path) {
			continue
		}
		rest := target.path[len(source.path):]
		copied := slot{region: destination.region, path: joinSlotPath(destination.path, trimSlash(rest))}
		state.contents[copied] = set.clone()
		graph.remember(copied, set)
	}
	copySlotMetadata(state.backing, source, destination)
	// Unknown writes beneath the copied aggregate stay unknown in its
	// snapshot. Otherwise an old backing copy can restore overwritten fields.
	copySlotMetadata(state.clobbered, source, destination)
	if backing, rest, ok := graph.backingOf(state, source); ok {
		if _, direct := state.backing[destination]; !direct {
			state.backing[destination] = graph.snapshotBeneath(backing, rest)
		}
	}
}

// copySlotMetadata rebases backing-copy identities or unknown-write stamps.
// Contents use their own copy path because pointee sets must be detached.
func copySlotMetadata[T any](entries map[slot]T, source, destination slot) {
	for target, value := range entries {
		if target.region != source.region || !slotBeneath(target.path, source.path) {
			continue
		}
		rest := target.path[len(source.path):]
		entries[slot{region: destination.region, path: joinSlotPath(destination.path, trimSlash(rest))}] = value
	}
}

// snapshotBeneath names the sub-aggregate of a snapshot at path as a
// snapshot of its own, so a copy of part of a copy still resolves.
func (graph *regionGraph) snapshotBeneath(snapshot *region, path string) *region {
	if path == "" {
		return snapshot
	}
	nested := graph.intern(regionKey{kind: regionSnapshot, origin: snapshot.origin, source: slot{region: snapshot, path: path}})
	nested.source = slot{region: snapshot, path: path}
	return nested
}

func trimSlash(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path[1:]
	}
	return path
}

// escapeInto names how a value stored into the object leaves local
// control: through a global, or through an object the caller can reach.
func escapeInto(object *region) HeapEscape {
	if object.kind == regionExternal {
		if _, global := object.origin.(*ssa.Global); global {
			return HeapEscapedGlobal
		}
	}
	return HeapEscapedField
}

// forgetWholeAbove drops the whole-aggregate entries of the slots above a
// sub-slot about to be written: the aggregate no longer equals the value
// stored into it whole.
func (graph *regionGraph) forgetWholeAbove(state *regionState, target slot) {
	if target.path == "" {
		return
	}
	for path := parentPath(target.path); ; path = parentPath(path) {
		delete(state.contents, slot{region: target.region, path: path})
		if path == "" {
			return
		}
	}
}

// storeAggregate records the aggregate value as the slot's whole content
// and copies its sub-slots beneath the target.
func (graph *regionGraph) storeAggregate(state *regionState, target slot, value pointees, strong bool, stamp int) {
	if !strong || len(value) != 1 || value.unknown() {
		// A possible aggregate replacement can change any stored field.
		// Its unknown stamp cannot override an old concrete field entry,
		// so forget the selected contents as well as the enclosing cache.
		graph.forgetStoredSlot(state, target, stamp)
		return
	}
	graph.clearSubtree(state, target)
	graph.forgetWholeAbove(state, target)
	for source := range value {
		if source.region.kind == regionNil {
			// A definite zero value empties the selected storage; it must
			// not inherit the uncertainty of a possible aggregate overwrite.
			return
		}
		state.contents[target] = value.clone()
		graph.remember(target, value)
		graph.copySubtree(state, source, target)
		if _, direct := state.backing[target]; !direct {
			state.backing[target] = graph.snapshotBeneath(source.region, source.path)
		}
		if clobber, ok := graph.clobberedBeneath(state, source); ok {
			state.clobbered[target] = clobber
		}
	}
}

// forgetStoredSlot replaces selected concrete storage with an unknown write.
// Former pointees, earlier snapshots and sibling storage are not modified.
func (graph *regionGraph) forgetStoredSlot(state *regionState, target slot, stamp int) {
	graph.forgetWholeAbove(state, target)
	graph.clearSubtree(state, target)
	state.clobbered[target] = stamp
}

// remember records, for the whole build, that the slot was given the value.
// History is used only for may-answers. Collapse an overfull set to unknown:
// retaining every object from repeated summary applications can otherwise
// dwarf the bounded current-state contents during a fixpoint.
func (graph *regionGraph) remember(target slot, value pointees) {
	held, ok := graph.history[target]
	if !ok {
		held = pointees{}
		graph.history[target] = held
	}
	if held.unknown() {
		return
	}
	for object, stale := range value {
		held.add(object, stale)
		if len(held) > pointeeLimit {
			clear(held)
			held[slot{region: graph.unkR}] = false
			return
		}
	}
}

// weakElementStore unions the value into the wildcard element slot and
// every constant element beneath the same array.
func (graph *regionGraph) weakElementStore(state *regionState, target slot, value pointees) {
	// A possible element write still invalidates an exact cached aggregate
	// above it. Keeping that whole value would hide the updated element set.
	graph.forgetWholeAbove(state, target)
	graph.remember(target, value)
	star := state.contents[target]
	if star == nil {
		// An unknown index may leave any particular element untouched.
		// Preserve existing elements and the unwritten nil/foreign possibility
		// before adding the new value; a wildcard store is never an exact fill.
		star = graph.content(state, target)
		state.contents[target] = star
	}
	star.union(value)
	graph.bound(state, target, nil)
	parent := parentPath(target.path)
	for other, set := range state.contents {
		if other.region == target.region && other.path != target.path && parentPath(other.path) == parent && isIndexStep(lastStep(other.path)) {
			set.union(value)
			graph.bound(state, other, nil)
		}
	}
}

// clearSubtree forgets everything beneath a slot before it is overwritten.
func (graph *regionGraph) clearSubtree(state *regionState, target slot) {
	state.forgetStoredSubtree(target)
	for other := range state.clobbered {
		if other.region == target.region && slotBeneath(other.path, target.path) {
			delete(state.clobbered, other)
		}
	}
}

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
		graph.storeSlot(state, target, value, strong, at)
	}
}

// storeSlot updates one selected scalar slot. SSA writes and imported edges
// share replacement, uncertainty, history and aggregate-cache invalidation;
// their callers retain destination selection and exposure policy.
func (graph *regionGraph) storeSlot(state *regionState, target slot, value pointees, strong bool, at ssa.Instruction) {
	if lastStep(target.path) == pathStar {
		graph.weakElementStore(state, target, value)
		return
	}
	graph.remember(target, value)
	graph.forgetWholeAbove(state, target)
	if strong {
		graph.clearSubtree(state, target)
		state.contents[target] = value.clone()
		return
	}
	existing, ok := state.contents[target]
	if !ok {
		existing = graph.content(state, target)
		state.contents[target] = existing
	}
	existing.union(value)
	graph.bound(state, target, at)
}

// By-value type traversal follows struct fields, array elements and SSA tuples.
// Reference edges remain leaves; the caller decides what matching a type means.
func anyByValueType(value types.Type, matches func(types.Type) bool) bool {
	if matches(value) {
		return true
	}
	switch value := value.Underlying().(type) {
	case *types.Struct:
		for field := range value.Fields() {
			if anyByValueType(field.Type(), matches) {
				return true
			}
		}
	case *types.Array:
		return anyByValueType(value.Elem(), matches)
	case *types.Tuple:
		for variable := range value.Variables() {
			if anyByValueType(variable.Type(), matches) {
				return true
			}
		}
	}
	return false
}

// walkStructReferences enumerates bounded reference slots in a by-value struct.
// Pointers stay leaves. Arrays and exhausted limits invoke cut so both snapshot
// construction and summary projection retain unknown evidence beyond the bound.
func walkStructReferences(typ types.Type, visit, cut func(string)) {
	count := 0
	var fields func(types.Type, string, int)
	fields = func(typ types.Type, path string, depth int) {
		if !tracked(typ) {
			return
		}
		if depth > SummaryPaths || count >= SummarySlots {
			cut("")
			return
		}
		switch typed := typ.Underlying().(type) {
		case *types.Struct:
			for index := range typed.NumFields() {
				fields(typed.Field(index).Type(), joinSlotPath(path, "field:"+strconv.Itoa(index)), depth+1)
			}
		case *types.Array:
			cut(path)
		default:
			count++
			visit(path)
		}
	}
	fields(typ, "", 0)
}

// Fixed views retain an offset into a local array; no backing-store alias set
// is inferred for arbitrary slice values. Bounds are checked before translating
// an element so different windows cannot silently name the same slot.
type storageArrayView struct {
	base              storageLocation
	offset, size, cap int64
}

func (storage *Storage) indexLocation(index *ssa.IndexAddr) (storageLocation, bool) {
	position, fixed := ssaflow.StorageInteger(index.Index, 0)
	view, known := storage.arrayView(index.X)
	if !fixed || !known || position < 0 || position >= view.size {
		return storageLocation{}, false
	}
	view.base.path += "/i" + strconv.FormatInt(view.offset+position, 10)
	return view.base, true
}

func (storage *Storage) arrayView(value ssa.Value) (storageArrayView, bool) {
	if value == nil || !storage.budget.Spend() {
		return storageArrayView{}, false
	}
	if sliced, ok := value.(*ssa.Slice); ok {
		base, known := storage.arrayView(sliced.X)
		low, lowOK := ssaflow.StorageInteger(sliced.Low, 0)
		high, highOK := ssaflow.StorageInteger(sliced.High, base.size)
		max, maxOK := ssaflow.StorageInteger(sliced.Max, base.cap)
		if !known || !lowOK || !highOK || !maxOK || low < 0 || low > high || high > max || max > base.cap {
			return storageArrayView{}, false
		}
		base.offset += low
		base.size, base.cap = high-low, max-low
		return base, true
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	if !ok {
		return storageArrayView{}, false
	}
	array, ok := pointer.Elem().Underlying().(*types.Array)
	if !ok {
		return storageArrayView{}, false
	}
	base, known := storage.location(value)
	return storageArrayView{base: base, size: array.Len(), cap: array.Len()}, known
}
