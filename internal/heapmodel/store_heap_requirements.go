package heapmodel

import "github.com/kojah/gohawk/internal/ssaflow"

import (
	"go/token"
	"go/types"
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

// heapRequirementLimit bounds the requirements one summary carries.
const heapRequirementLimit = 16

// heapRequirementProofLimit bounds the distinct candidate proofs attempted
// before projecting a summary. Omitted requirements make no guarantee, so a
// limit can lose coverage but cannot invent a caller precondition.
const heapRequirementProofLimit = 64

// heapRequirementStateBudget bounds the path states expanded for one candidate.
// A cut gives no requirement; the caller must not read missing requirements
// as proof that the callee does nothing.
const heapRequirementStateBudget = 1000

type requirementKey struct {
	slot   slot
	kind   HeapRequirementKind
	method string
}

type requirementCandidate struct {
	key         requirementKey
	requirement HeapRequirement
}

// requirements projects the every-return method calls on named slots.
func (projection *heapProjection) requirements() []HeapRequirement {
	calls := map[ssa.Instruction][]requirementKey{}
	keys := map[requirementKey]bool{}
	for _, block := range projection.graph.function.Blocks {
		for _, instruction := range block.Instrs {
			for _, key := range projection.instructionRequirements(instruction) {
				calls[instruction] = append(calls[instruction], key)
				keys[key] = true
			}
		}
	}
	truncated := map[HeapSlot]bool{}
	for at := range projection.cuts {
		truncated[at] = true
	}
	var candidates []requirementCandidate
	for key := range keys {
		named, ok := projection.rootOf(key.slot.region)
		if !ok {
			continue
		}
		at := HeapSlot{Root: named.Root, Path: joinSlotPath(named.Path, key.slot.path)}
		if truncated[HeapSlot{Root: at.Root}] || len(ssaflow.SplitAccessPath(at.Path)) > SummaryPaths {
			continue
		}
		candidates = append(candidates, requirementCandidate{
			key: key, requirement: HeapRequirement{Slot: at, Kind: key.kind, Method: key.method},
		})
	}
	return boundedRequirements(candidates, func(key requirementKey) bool {
		return onEveryReturn(projection.graph.function, ssaflow.NewSearchBudget(heapRequirementStateBudget), func(instruction ssa.Instruction) bool {
			return slices.Contains(calls[instruction], key)
		})
	})
}

// boundedRequirements proves candidates in summary order, stopping once the
// published limit is full or the proof-work limit is spent. Sorting first
// keeps both the selected guarantees and the coverage loss deterministic.
func boundedRequirements(candidates []requirementCandidate, proves func(requirementKey) bool) []HeapRequirement {
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.requirement.Slot != right.requirement.Slot {
			return SlotLess(left.requirement.Slot, right.requirement.Slot)
		}
		if left.requirement.Kind != right.requirement.Kind {
			return left.requirement.Kind < right.requirement.Kind
		}
		if left.requirement.Method != right.requirement.Method {
			return left.requirement.Method < right.requirement.Method
		}
		if left.key.slot.region.serial != right.key.slot.region.serial {
			return left.key.slot.region.serial < right.key.slot.region.serial
		}
		return left.key.slot.path < right.key.slot.path
	})
	var requirements []HeapRequirement
	for index, candidate := range candidates {
		if index >= heapRequirementProofLimit || len(requirements) == heapRequirementLimit {
			break
		}
		if !proves(candidate.key) {
			continue
		}
		requirements = append(requirements, candidate.requirement)
	}
	return requirements
}

// instructionRequirements names what one instruction requires of the
// objects it touches. A dereference requires its object non-nil: a load
// or store through a pointer, a field or element selected beneath one,
// and a method invoked through an interface. A method called on a pointer
// receiver requires only the method, because Go lets a nil pointer
// receive it. A call to a summarized callee requires what the callee
// requires of the arguments it was handed. An object the graph cannot
// resolve to one slot requires nothing, because the instruction would not
// prove which object it relies on.
func (projection *heapProjection) instructionRequirements(instruction ssa.Instruction) []requirementKey {
	switch typed := instruction.(type) {
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			return projection.nonNil(typed.X)
		}
	case *ssa.Store:
		return projection.nonNil(typed.Addr)
	case *ssa.FieldAddr:
		return projection.nonNil(typed.X)
	case *ssa.IndexAddr:
		if _, ok := typed.X.Type().Underlying().(*types.Pointer); ok {
			return projection.nonNil(typed.X)
		}
	case *ssa.Call:
		return projection.callRequirements(typed)
	}
	return nil
}

// nonNil is the non-nil requirement on the one object a pointer refers
// into. An interior pointer, the address of a field or element, is as
// non-nil as the object it selects from, so the requirement is on the
// object and not on the slot: dereferencing &resp.next requires resp.
func (projection *heapProjection) nonNil(value ssa.Value) []requirementKey {
	target, ok := singleSlot(projection.graph.pointees(value))
	if !ok || target.region.kind == regionNil {
		return nil
	}
	return []requirementKey{{slot: slot{region: target.region}, kind: HeapRequiresNonNil}}
}

// callRequirements names what one call requires: a method call whose
// receiver is exactly one object, which an interface invocation also
// requires non-nil, or a summarized callee's requirements mapped onto the
// arguments it was handed.
func (projection *heapProjection) callRequirements(call *ssa.Call) []requirementKey {
	common := call.Common()
	keys := projection.receiverRequirements(common)
	callee := common.StaticCallee()
	if callee == nil || common.IsInvoke() || sameCallCycle(projection.graph.function, callee) {
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
		target := projection.throughLocal(slot{region: base.region, path: joinSlotPath(base.path, requirement.Slot.Path)}, call)
		keys = append(keys, requirementKey{slot: target, kind: requirement.Kind, method: requirement.Method})
	}
	return keys
}

// throughLocal follows a required slot beneath an object the function
// allocated to the object that slot certainly holds at the call, so a
// callee's requirement on w.r, where w wraps a parameter, becomes one on
// the parameter. Each step must hold exactly one non-stale object there;
// a wrapper field reassigned, set on one branch only, or handed to
// unknown code first holds none, and the slot is kept as it was, where it
// names no root and requires nothing of the caller's caller.
func (projection *heapProjection) throughLocal(target slot, call ssa.Instruction) slot {
	if target.region.kind != regionSite || target.path == "" {
		return target
	}
	state := projection.graph.stateAt(call)
	if state == nil {
		return target
	}
	steps := ssaflow.SplitAccessPath(target.path)
	current := slot{region: target.region}
	for index, step := range steps {
		current.path = joinSlotPath(current.path, step)
		held, ok := singleSlot(projection.graph.content(state, current))
		if !ok || held.region.kind == regionNil {
			return target
		}
		rest := ssaflow.JoinAccessPath(steps[index+1:])
		if held.region.kind != regionSite {
			return slot{region: held.region, path: joinSlotPath(held.path, rest)}
		}
		current = held
	}
	return target
}

// receiverRequirements names what a method call requires of its receiver:
// the method, and non-nil when the receiver is an interface.
func (projection *heapProjection) receiverRequirements(common *ssa.CallCommon) []requirementKey {
	name := ssaflow.CallName(common)
	receiver := ssaflow.CallReceiver(common)
	if name == "" || receiver == nil {
		return nil
	}
	target, ok := singleSlot(projection.graph.pointees(receiver))
	if !ok {
		return nil
	}
	keys := []requirementKey{{slot: target, kind: HeapRequiresMethod, method: name}}
	if common.IsInvoke() {
		keys = append(keys, requirementKey{slot: target, kind: HeapRequiresNonNil})
	}
	return keys
}

// onEveryReturn reports whether an instruction the predicate accepts
// precedes every normal return: the every-return polarity of the
// completion search, asked of the flow directly. A function that never
// returns, or never makes such a call, requires nothing.
func onEveryReturn(function *ssa.Function, budget *ssaflow.SearchBudget, calls func(ssa.Instruction) bool) bool {
	hasReturn, hasCall := false, false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
			hasCall = hasCall || calls(instruction)
		}
	}
	if !hasReturn || !hasCall {
		return false
	}
	flow := ssaflow.ObligationFlow{Instruction: ssaflow.ExactOrNone(calls), Budget: budget}
	return ssaflow.EvaluateObligationFromEntry(function, flow) == ssaflow.ObligationHonored
}
