package lifecyclefacts

import (
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"

	"golang.org/x/tools/go/ssa"
)

// The heap projection is the one encoding of what a function does to the
// objects a caller can name. The graph supplies the edges, escapes, reads,
// and truncation; the every-return release proofs already computed for the
// discharge claims supply the release effects, because release is a
// coverage question the flow-sensitive graph does not answer on its own. A
// function whose graph is unavailable is projected as truncated at every
// parameter, so an importer applies it as an unresolved call rather than as
// a function with no effects.

// projectHeap projects the function's heap, truncated at every parameter
// and result when the graph could not.
func projectHeap(function *ssa.Function) *heapmodel.HeapSummary {
	summary, ok := heapmodel.ProjectHeap(function)
	if !ok {
		for index := range function.Params {
			summary.Truncated = append(summary.Truncated, heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index}})
		}
		for index := range function.Signature.Results().Len() {
			summary.Truncated = append(summary.Truncated, heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapResult, Index: index}})
		}
	}
	return &summary
}

// withReleases adds the release effects the discharge proofs established.
func withReleases(summary *heapmodel.HeapSummary, fact *Fact) *heapmodel.HeapSummary {
	for _, discharge := range fact.unconditionalDischarges() {
		// Calling a function parameter releases nothing the heap tracks.
		if discharge.Method == InvokeMethod || discharge.Method == SynchronousInvokeMethod {
			continue
		}
		summary.Effects = append(summary.Effects, heapmodel.HeapEffect{
			Slot:    heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: discharge.Parameter}, Path: discharge.Path},
			Release: discharge.Method,
			Every:   true,
		})
	}
	return summary
}

// returnsOwner is the ReturnedOwner claim as a query over the projection:
// on every normal return with a non-nil result, some result, or a slot
// beneath one, holds the parameter's object and nothing else. The
// projection judges result roots only on returns where the result is not
// nil, so a constructor that returns nil beside an error on failure still
// qualifies.
func returnsOwner(summary *heapmodel.HeapSummary, index int) bool {
	for _, hold := range summary.Holds {
		if hold.Parameter == index && hold.Must {
			return true
		}
	}
	return false
}

// retained is the Retained claim as a query over the projection, and keeps
// its loose polarity: the parameter's object left local control in any
// way, or some result or global may hold it.
func retained(summary *heapmodel.HeapSummary, index int) bool {
	parameter := heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index}}
	for _, effect := range summary.Effects {
		if effect.Slot == parameter && effect.Escape != 0 {
			return true
		}
	}
	return heldOutside(summary, parameter, true)
}

// stored is the Stored claim as a query over the projection, and keeps its
// strict polarity: positive evidence that the parameter's object was put
// somewhere that outlives the call, a global, an object the caller can
// reach, a channel, or a package variable's slot. Handing it to a call or a
// goroutine, or returning it, is not storage.
func stored(summary *heapmodel.HeapSummary, index int) bool {
	parameter := heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index}}
	for _, effect := range summary.Effects {
		if effect.Slot == parameter && effect.Escape&(heapmodel.HeapEscapedGlobal|heapmodel.HeapEscapedField|heapmodel.HeapEscapedSend) != 0 {
			return true
		}
	}
	return heldOutside(summary, parameter, false)
}

// heldOutside reports whether a global's slot, or with results also a
// result's slot, may hold the object at the slot.
func heldOutside(summary *heapmodel.HeapSummary, slot heapmodel.HeapSlot, results bool) bool {
	for _, edge := range summary.Edges {
		if edge.To.Kind != heapmodel.HeapTargetSlot || edge.To.Slot != slot {
			continue
		}
		if edge.From.Root.Kind == heapmodel.HeapGlobal || results && edge.From.Root.Kind == heapmodel.HeapResult {
			return true
		}
	}
	return false
}

// kept is the Kept claim as a query over the projection: every path
// beneath the parameter whose content left local control or may be held by
// a result or global. Content the projection could not name that is held
// outside is claimed at the parameter itself, because it may have come
// from anywhere beneath it. A truncated root is not a kept claim: an
// unresolved callee keeps only what it was handed, which the escape
// effects record.
func kept(summary *heapmodel.HeapSummary, index int) []Kept {
	parameter := heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index}}
	seen := map[string]bool{}
	var claims []Kept
	claim := func(path string) {
		if !seen[path] {
			seen[path] = true
			claims = append(claims, Kept{Parameter: index, Path: path})
		}
	}
	for _, effect := range summary.Effects {
		if effect.Escape != 0 && effect.Slot.Root == parameter.Root && effect.Slot.Path != "" {
			claim(effect.Slot.Path)
		}
	}
	for _, edge := range summary.Edges {
		outside := edge.From.Root.Kind == heapmodel.HeapGlobal || edge.From.Root.Kind == heapmodel.HeapResult
		if !outside {
			continue
		}
		switch {
		case edge.To.Kind == heapmodel.HeapTargetUnknown:
			claim("")
		case edge.To.Kind == heapmodel.HeapTargetSlot && edge.To.Slot.Root == parameter.Root && edge.To.Slot.Path != "":
			claim(edge.To.Slot.Path)
		}
	}
	slices.SortFunc(claims, func(left, right Kept) int { return strings.Compare(left.Path, right.Path) })
	return claims
}

// receiverStores is the ReceiverStore claim as a query over the projection:
// on every normal return, some slot beneath the receiver's object holds the
// parameter's object and nothing else. A store of a wrapper around the
// parameter counts when the wrapper's slot holding it is in the projection,
// which a summarized wrapper constructor provides.
func receiverStores(summary *heapmodel.HeapSummary, index int) bool {
	parameter := heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index}}
	for _, edge := range summary.Edges {
		if edge.Must && edge.From.Root.Kind == heapmodel.HeapParameter && edge.From.Root.Index == 0 && edge.From.Path != "" &&
			edge.To.Kind == heapmodel.HeapTargetSlot && edge.To.Slot == parameter {
			return true
		}
	}
	return false
}

// ArgumentMethodsRequired lists the methods the call's static callee is
// summarized as calling, on every normal return, with the object passed at
// index as the receiver. The names are the callee's own; the caller decides
// what they mean for the concrete value it passed. A callee without a
// summary requires nothing here, which a consumer must read as unknown.
func (evidence *LifecycleEvidence) ArgumentMethodsRequired(instruction ssa.Instruction, index int) []string {
	fact, ok := factFor(evidence.pass, instruction)
	if !ok || fact.Heap == nil {
		return nil
	}
	var methods []string
	for _, requirement := range fact.Heap.Requires {
		if requirement.Kind == heapmodel.HeapRequiresMethod && requirement.Slot.Root.Kind == heapmodel.HeapParameter &&
			requirement.Slot.Root.Index == index && requirement.Slot.Path == "" {
			methods = append(methods, requirement.Method)
		}
	}
	return methods
}

// The transfer and retention claims are read from the heap projection rather
// than stored beside it, so the claims a consumer reads and the summary a
// caller's graph applies are one record. The projection does not know the
// signature, so each claim applies the type gates here: only a parameter
// that can hold an object has a claim, an error result never owns one, and
// only a struct-shaped parameter has kept contents. A fact read without its
// function, such as a hand-built one, claims none of them.

// ReturnedOwner marks parameters that some non-error result holds on every
// normal return with a non-nil result.
func (fact *Fact) ReturnedOwner() ParameterMask {
	if fact.signature == nil || !canReturnOwner(fact.signature.Results()) {
		return 0
	}
	return fact.heapClaim(func(index int) bool { return returnsOwner(fact.Heap, index) })
}

// ReceiverStore marks parameters kept in a slot beneath the receiver.
func (fact *Fact) ReceiverStore() ParameterMask {
	return fact.heapClaim(func(index int) bool { return index > 0 && receiverStores(fact.Heap, index) })
}

// Retained marks parameters the callee may keep beyond the call. It is a
// may-claim; see retention.go for the over-approximation it makes.
func (fact *Fact) Retained() ParameterMask {
	return fact.heapClaim(func(index int) bool { return retained(fact.Heap, index) })
}

// Stored is positive structural evidence that the callee keeps the
// parameter, safe to treat as an ownership transfer.
func (fact *Fact) Stored() ParameterMask {
	return fact.heapClaim(func(index int) bool { return stored(fact.Heap, index) })
}

// Kept widens Retained to what is loaded out of a struct-shaped parameter,
// by access path, so a caller can ask whether the resource it stored at one
// path may outlive the call. A retained parameter already keeps all of it.
func (fact *Fact) Kept() []Kept {
	var claims []Kept
	for index, parameter := range fact.parameterTypes() {
		if ownershipCapableType(parameter) && structShaped(parameter) && !retained(fact.Heap, index) {
			claims = append(claims, kept(fact.Heap, index)...)
		}
	}
	return claims
}

func (fact *Fact) heapClaim(holds func(int) bool) ParameterMask {
	var mask ParameterMask
	for index, parameter := range fact.parameterTypes() {
		if ownershipCapableType(parameter) && holds(index) {
			mask |= parameterMaskFor(index)
		}
	}
	return mask
}

// parameterTypes lists the types at SSA parameter positions, the receiver
// first, or nothing when the fact has no heap or no signature to read.
func (fact *Fact) parameterTypes() []types.Type {
	if fact.Heap == nil || fact.signature == nil {
		return nil
	}
	var parameters []types.Type
	if receiver := fact.signature.Recv(); receiver != nil {
		parameters = append(parameters, receiver.Type())
	}
	for parameter := range fact.signature.Params().Variables() {
		parameters = append(parameters, parameter.Type())
	}
	if len(parameters) > 64 {
		return nil
	}
	return parameters
}
