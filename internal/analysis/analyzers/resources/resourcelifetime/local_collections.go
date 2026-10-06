package resourcelifetime

import (
	"slices"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
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
	// releasingCalls are calls that hand the whole collection to a helper
	// that releases every element of it on every normal return.
	releasingCalls map[ssa.Instruction]bool
	evidence       *lifecyclefacts.LifecycleEvidence
}

// collectionDecision is the model's answer for one resource: the collection
// when every use is understood, or the instruction that declined it.
type collectionDecision struct {
	collection *localCollection
	declinedAt ssa.Instruction
}

func findLocalCollection(
	evidence *lifecyclefacts.LifecycleEvidence, resource ssa.Value, cleanup []string, budget *proofs.SearchBudget,
) collectionDecision {
	appends := resourceAppends(resource)
	if len(appends) == 0 {
		return collectionDecision{}
	}
	collection := &localCollection{
		appends: appends, versions: ssaflow.SliceVersions(appends[0]), released: map[[2]*ssa.BasicBlock]bool{},
		releasingCalls: map[ssa.Instruction]bool{}, evidence: evidence,
	}
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
func (collection *localCollection) understood(version ssa.Value, user ssa.Instruction, cleanup []string, budget *proofs.SearchBudget) bool {
	switch typed := user.(type) {
	case *ssa.Phi:
		return collection.member(typed)
	case *ssa.DebugRef, *ssa.Return:
		return true
	case *ssa.Call:
		if collection.member(typed) {
			return true
		}
		if builtin, ok := typed.Call.Value.(*ssa.Builtin); ok {
			return builtin.Name() == "len" || builtin.Name() == "cap"
		}
		return collection.releasedByHelper(typed, version, cleanup)
	case *ssa.IndexAddr:
		return typed.X == version && collection.releaseLoopReads(typed, cleanup, budget)
	}
	return false
}

// releaseLoopReads reports whether an element address is the element read of
// a range loop over the version that releases that element on every
// iteration, and records the loop's exit edge as releasing the collection.
func (collection *localCollection) releaseLoopReads(address *ssa.IndexAddr, cleanup []string, budget *proofs.SearchBudget) bool {
	for _, header := range address.Parent().Blocks {
		loop, ok := ssaflow.RangeElementLoop(header, budget)
		if !ok || !loop.ReadsElement(address) || !lifecycle.ElementLoopReleasesEach(loop, address, cleanup) {
			continue
		}
		collection.released[[2]*ssa.BasicBlock{loop.Loop.Header, loop.Done}] = true
		return true
	}
	return false
}

// releasedByHelper reports whether the call hands this version of the
// collection, whole and exactly once, to a helper that releases every element
// of that argument on every normal return, and records the call as releasing
// the collection. A sub-slice or a copy is another value and declines the
// model before it reaches here.
func (collection *localCollection) releasedByHelper(call *ssa.Call, version ssa.Value, cleanup []string) bool {
	index := slices.Index(call.Call.Args, version)
	if index < 0 || slices.Index(call.Call.Args[index+1:], version) >= 0 || collection.evidence == nil ||
		!collection.evidence.ReleasesEachElement(call, index, cleanup) {
		return false
	}
	collection.releasingCalls[call] = true
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
		if collection.releasingCalls[typed] {
			return actionSettled, resourceReasonCollectionReleasedByHelper, true
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
