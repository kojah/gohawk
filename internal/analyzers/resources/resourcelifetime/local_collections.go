package resourcelifetime

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Local collections. A resource appended to a slice this function created
// stays this function's to release, through the slice, when every use of
// every version of the slice is understood: further appends, the phis a loop
// merges them in, its length, returning it whole, and reading its elements
// only in range loops that release each one. Such a loop settles the
// resource on its exit edge, whatever the length: every iteration released
// the element it read, and together the iterations read them all. Returning
// the slice hands the resource to the caller.
//
// Any other use of the slice, such as storing, passing, slicing, copying,
// sending, or capturing it, or reading an element anywhere else, declines
// the model, and the append stays unknown as it was before. Declining must
// be all or nothing: a loop that reads elements without provably releasing
// them may still release this one, so it cannot be left to the walk as an
// unrelated instruction, or the path that skips the loop would read as a
// leak. The model therefore only turns an unknown into a proof, or into a
// leak when the slice is dropped still holding the resource.

type localCollection struct {
	appends  []*ssa.Call
	versions []ssa.Value
	// released holds the exit edges of the loops that release every element.
	released map[[2]*ssa.BasicBlock]bool
}

// collectionDecision is the model's answer for one resource: the collection
// when every use is understood, or the instruction that declined it.
type collectionDecision struct {
	collection *localCollection
	declinedAt ssa.Instruction
}

func findLocalCollection(resource ssa.Value, cleanup []string, budget *ssaflow.SearchBudget) collectionDecision {
	appends := resourceAppends(resource)
	if len(appends) == 0 {
		return collectionDecision{}
	}
	collection := &localCollection{appends: appends, versions: ssaflow.SliceVersions(appends[0]), released: map[[2]*ssa.BasicBlock]bool{}}
	// Every append of the resource must feed this one collection.
	for _, call := range appends[1:] {
		if !slices.Contains(collection.versions, ssa.Value(call)) {
			return collectionDecision{declinedAt: call}
		}
	}
	for _, version := range collection.versions {
		if !collection.freshOrigin(version) {
			return collectionDecision{declinedAt: version.(ssa.Instruction)}
		}
		for _, user := range *version.Referrers() {
			if !collection.understood(version, user, cleanup, budget) {
				return collectionDecision{declinedAt: user}
			}
		}
	}
	return collectionDecision{collection: collection}
}

// resourceAppends returns the appends that add the resource itself, or the
// resource converted to an interface, as a separate argument.
func resourceAppends(resource ssa.Value) []*ssa.Call {
	var appends []*ssa.Call
	for _, block := range resource.Parent().Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			values, ok := ssaflow.AppendedValues(call)
			if ok && slices.ContainsFunc(values, func(value ssa.Value) bool { return appendedResource(value, resource) }) {
				appends = append(appends, call)
			}
		}
	}
	return appends
}

// appendedResource reports whether an appended value is the resource, or the
// resource converted to an interface for an interface-typed slice.
func appendedResource(value, resource ssa.Value) bool {
	if boxed, ok := value.(*ssa.MakeInterface); ok {
		value = boxed.X
	}
	return value == resource
}

// freshOrigin reports whether every value flowing into a version from
// outside the collection is a slice this function made empty: nil, or make.
// A slice received from a caller or loaded from storage may share its
// backing array with someone else.
func (collection *localCollection) freshOrigin(version ssa.Value) bool {
	switch typed := version.(type) {
	case *ssa.Phi:
		for _, incoming := range ssaflow.PhiIncoming(typed) {
			if !collection.member(incoming) && !freshSlice(incoming) {
				return false
			}
		}
		return true
	case *ssa.Call:
		base := typed.Call.Args[0]
		return collection.member(base) || freshSlice(base)
	}
	return false
}

func freshSlice(value ssa.Value) bool {
	switch typed := value.(type) {
	case *ssa.Const:
		return typed.IsNil()
	case *ssa.MakeSlice:
		return true
	}
	return false
}

func (collection *localCollection) member(value ssa.Value) bool {
	return slices.Contains(collection.versions, value)
}

// understood reports whether one use of a version keeps the collection's
// elements where this function can account for them.
func (collection *localCollection) understood(version ssa.Value, user ssa.Instruction, cleanup []string, budget *ssaflow.SearchBudget) bool {
	switch typed := user.(type) {
	case *ssa.Phi:
		return collection.member(typed)
	case *ssa.DebugRef, *ssa.Return:
		return true
	case *ssa.Call:
		if collection.member(typed) {
			return true
		}
		builtin, ok := typed.Call.Value.(*ssa.Builtin)
		return ok && (builtin.Name() == "len" || builtin.Name() == "cap")
	case *ssa.IndexAddr:
		return typed.X == version && collection.releaseLoopReads(typed, cleanup, budget)
	}
	return false
}

// releaseLoopReads reports whether an element address is the element read of
// a range loop over the version that releases that element on every
// iteration, and records the loop's exit edge as releasing the collection.
func (collection *localCollection) releaseLoopReads(address *ssa.IndexAddr, cleanup []string, budget *ssaflow.SearchBudget) bool {
	for _, header := range address.Parent().Blocks {
		loop, ok := ssaflow.RangeElementLoop(header, budget)
		if !ok || !loop.ReadsElement(address) || !releasesEachElement(loop, address, cleanup) {
			continue
		}
		collection.released[[2]*ssa.BasicBlock{loop.Loop.Header, loop.Done}] = true
		return true
	}
	return false
}

// releasesEachElement reports whether the element read at address is used
// only as the receiver of a cleanup call that runs on every iteration: its
// block dominates every back edge to the header.
func releasesEachElement(loop ssaflow.ElementLoop, address *ssa.IndexAddr, cleanup []string) bool {
	released := false
	for _, user := range *address.Referrers() {
		if _, ok := user.(*ssa.DebugRef); ok {
			continue
		}
		load, ok := user.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			return false
		}
		for _, use := range *load.Referrers() {
			if _, ok := use.(*ssa.DebugRef); ok {
				continue
			}
			call, ok := use.(*ssa.Call)
			if !ok || ssaflow.CallReceiver(call.Common()) != load || !slices.Contains(cleanup, ssaflow.CallName(call.Common())) ||
				!runsEveryIteration(loop, call.Block()) {
				return false
			}
			released = true
		}
	}
	return released
}

func runsEveryIteration(loop ssaflow.ElementLoop, block *ssa.BasicBlock) bool {
	for _, predecessor := range loop.Loop.Header.Preds {
		if loop.Loop.Contains(predecessor) && !block.Dominates(predecessor) {
			return false
		}
	}
	return true
}

// label is the classifier's answer for an instruction the collection
// explains: an append that keeps the resource owned, and a return that hands
// the whole collection to the caller.
func (collection *localCollection) label(instruction ssa.Instruction) (resourceAction, resourceLifetimeReason, bool) {
	if collection == nil {
		return actionNone, resourceReasonNone, false
	}
	switch typed := instruction.(type) {
	case *ssa.Call:
		if slices.Contains(collection.appends, typed) {
			return actionNone, resourceReasonAppendedToLocalCollection, true
		}
	case *ssa.Return:
		if slices.ContainsFunc(typed.Results, collection.member) {
			return actionSettled, resourceReasonCollectionReturned, true
		}
	}
	return actionNone, resourceReasonNone, false
}

// releasedOnEdge reports whether the edge leaves a loop that released every
// element of the collection.
func (collection *localCollection) releasedOnEdge(from, to *ssa.BasicBlock) bool {
	return collection != nil && collection.released[[2]*ssa.BasicBlock{from, to}]
}
