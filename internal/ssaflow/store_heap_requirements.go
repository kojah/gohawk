package ssaflow

import (
	"slices"
	"sort"

	"golang.org/x/tools/go/ssa"
)

// Requirements are the precondition half of a heap summary: facts about
// the objects a function was handed that held on every path to a normal
// return. The first family records the methods the function calls on the
// object at a named slot, directly or through a summarized callee, with
// the callee's own method name. Whether that method is an operation the
// object's contract forbids after release is the caller's decision, made
// from the concrete type of the argument it passed; the summary only says
// what was called. A requirement is never projected for a slot the
// projection truncated, and only when the call happens on every normal
// return, because the summary is joined over paths and a fact that holds
// on some path only is not a fact the caller can be held to.

// HeapRequirement says the function calls Method with the object at Slot
// as the receiver, on every normal return.
type HeapRequirement struct {
	Slot   HeapSlot
	Method string
}

// heapRequirementLimit bounds the requirements one summary carries.
const heapRequirementLimit = 16

type requirementKey struct {
	slot   slot
	method string
}

// requirements projects the every-return method calls on named slots.
func (projection *heapProjection) requirements() []HeapRequirement {
	calls := map[ssa.Instruction][]requirementKey{}
	keys := map[requirementKey]bool{}
	for _, block := range projection.graph.function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			for _, key := range projection.callRequirements(call) {
				calls[instruction] = append(calls[instruction], key)
				keys[key] = true
			}
		}
	}
	truncated := map[HeapSlot]bool{}
	for at := range projection.cuts {
		truncated[at] = true
	}
	var requirements []HeapRequirement
	for key := range keys {
		named, ok := projection.rootOf(key.slot.region)
		if !ok {
			continue
		}
		at := HeapSlot{Root: named.Root, Path: joinSlotPath(named.Path, key.slot.path)}
		if truncated[HeapSlot{Root: at.Root}] || len(SplitAccessPath(at.Path)) > heapPathDepth {
			continue
		}
		if !onEveryReturn(projection.graph.function, func(instruction ssa.Instruction) bool {
			return slices.Contains(calls[instruction], key)
		}) {
			continue
		}
		requirements = append(requirements, HeapRequirement{Slot: at, Method: key.method})
	}
	sort.Slice(requirements, func(i, j int) bool {
		if requirements[i].Slot != requirements[j].Slot {
			return heapSlotLess(requirements[i].Slot, requirements[j].Slot)
		}
		return requirements[i].Method < requirements[j].Method
	})
	if len(requirements) > heapRequirementLimit {
		requirements = requirements[:heapRequirementLimit]
	}
	return requirements
}

// callRequirements names what one call requires: a method call whose
// receiver is exactly one object, or a summarized callee's requirements
// mapped onto the arguments it was handed. A receiver that may be several
// objects, or unknown, requires nothing, because the call would not prove
// which object it operates on.
func (projection *heapProjection) callRequirements(call *ssa.Call) []requirementKey {
	common := call.Common()
	var keys []requirementKey
	if name := CallName(common); name != "" {
		if receiver := CallReceiver(common); receiver != nil {
			if target, ok := singleSlot(projection.graph.pointees(receiver)); ok {
				keys = append(keys, requirementKey{slot: target, method: name})
			}
		}
	}
	callee := common.StaticCallee()
	if callee == nil || common.IsInvoke() {
		return keys
	}
	summary, ok := heapSummaryOf(callee)
	if !ok {
		return keys
	}
	for _, requirement := range summary.Requires {
		if requirement.Slot.Root.Kind != HeapParameter || requirement.Slot.Root.Index >= len(common.Args) {
			continue
		}
		base, ok := singleSlot(projection.graph.pointees(common.Args[requirement.Slot.Root.Index]))
		if !ok {
			continue
		}
		keys = append(keys, requirementKey{
			slot: slot{region: base.region, path: joinSlotPath(base.path, requirement.Slot.Path)}, method: requirement.Method,
		})
	}
	return keys
}

// onEveryReturn reports whether an instruction the predicate accepts
// precedes every normal return: the every-return polarity of the
// completion search, asked of the flow directly. A function that never
// returns, or never makes such a call, requires nothing.
func onEveryReturn(function *ssa.Function, calls func(ssa.Instruction) bool) bool {
	hasReturn, hasCall := false, false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
			hasCall = hasCall || calls(instruction)
		}
	}
	return hasReturn && hasCall && !UnownedReturnFromEntryAssumingNonNil(function, nil, calls)
}
