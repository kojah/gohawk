package heapmodel

import "github.com/kojah/gohawk/internal/ssaflow"

import (
	"strconv"

	"golang.org/x/tools/go/ssa"
)

// Applying a heap summary at a call site is substitution: the callee's
// parameter becomes the argument's slots, its result the call's, a global
// the same global, and fresh a new object owned by this call. Nothing else
// the caller holds is touched, which is the whole point: a summarized
// callee no longer makes the graph forget every object the caller did not
// allocate. A truncated root is forgotten beneath, exactly as an
// unresolved call would forget it.

// applyHeapSummary applies the callee's summary to the state and reports
// whether it did; a summary the call's shape cannot be matched to, such as
// a closure with captured variables, is not applied.
func (graph *regionGraph) applyHeapSummary(state *regionState, common *ssa.CallCommon, instruction ssa.Instruction) bool {
	callee := common.StaticCallee()
	if common.IsInvoke() {
		graph.recordCall(appliedSummary{instruction: instruction, reason: CallInterface})
		return false
	}
	if callee == nil {
		graph.recordCall(appliedSummary{instruction: instruction, reason: CallDynamic})
		return false
	}
	if sameCallCycle(graph.function, callee) {
		graph.recordCall(appliedSummary{instruction: instruction, callee: callee, reason: CallRecursive})
		return false
	}
	for _, key := range summaryKeys(callee) {
		if _, seen := graph.consulted[key]; !seen {
			graph.consulted[key] = heapSummaryGeneration(key)
		}
	}
	summary, ok := heapSummaryOf(callee)
	switch {
	case !ok:
		graph.recordCall(appliedSummary{instruction: instruction, callee: callee, reason: CallNoSummary})
		return false
	case len(callee.FreeVars) != 0:
		graph.recordCall(appliedSummary{instruction: instruction, callee: callee, reason: CallClosure})
		return false
	}
	call, isCall := instruction.(*ssa.Call)
	graph.recordCall(appliedSummary{
		instruction: instruction, callee: callee, reason: CallSummaryApplied,
		edges: len(summary.Edges), effects: len(summary.Effects), truncated: len(summary.Truncated),
	})
	substitution := &heapSubstitution{graph: graph, state: state, common: common, instruction: instruction, fresh: map[string]*region{}}
	graph.applyEscapes(state, summary, substitution, instruction)
	for _, at := range summary.Truncated {
		substitution.truncate(at)
	}
	// A result the summary names as one fresh object on every return is the
	// object its sub-slot edges describe; name it before those edges.
	for _, edge := range summary.Edges {
		if edge.Must && edge.From.Root.Kind == HeapResult && edge.From.Path == "" && edge.To.Kind == HeapTargetFresh {
			object := substitution.freshObject("fresh:" + edge.To.Origin + "#" + strconv.Itoa(edge.To.Object))
			substitution.fresh["result:"+strconv.Itoa(edge.From.Root.Index)] = object
		}
	}
	for _, edge := range summary.Edges {
		substitution.apply(edge)
	}
	if isCall {
		substitution.results(call)
	}
	return true
}

// applyEscapes applies the callee's escapes. They describe the objects as
// they were handed in, so they are applied before the truncation and the
// edges rewrite the caller's slots: an argument whose contents the callee
// may also have overwritten still escaped with everything it held.
func (graph *regionGraph) applyEscapes(state *regionState, summary HeapSummary, substitution *heapSubstitution, instruction ssa.Instruction) {
	for _, effect := range summary.Effects {
		if effect.Escape == 0 {
			continue
		}
		// An escape at the object itself escapes the object; one at a path
		// beneath it escapes what that slot holds.
		set := substitution.slots(effect.Slot)
		if effect.Slot.Path != "" {
			contents := pointees{}
			for held, stale := range set {
				for pointee, pointeeStale := range graph.content(state, held) {
					contents.add(pointee, stale || pointeeStale)
				}
			}
			set = contents
		}
		graph.escape(state, set, effect.Escape, instruction)
		// An object the callee handed to code nobody can follow, such as a
		// callback it invoked, may have been written through by that code
		// with everything it holds, closures' captured variables included;
		// the caller forgets it exactly as it would at its own unresolved
		// call. The summary's edges, applied afterwards, restore what the
		// callee proved.
		if effect.Escape&(HeapEscapedCall|HeapEscapedAsync) != 0 {
			graph.clobber(state, set, graph.id(instruction))
		}
	}
}

type heapSubstitution struct {
	graph       *regionGraph
	state       *regionState
	common      *ssa.CallCommon
	instruction ssa.Instruction
	// fresh interns the objects this call created, one per origin.
	fresh map[string]*region
	// results collects each result's pointees.
	returned map[int]pointees
}

// slots resolves a summary slot to the caller's slots.
func (substitution *heapSubstitution) slots(at HeapSlot) pointees {
	var base pointees
	switch at.Root.Kind {
	case HeapParameter:
		if at.Root.Index >= len(substitution.common.Args) {
			return pointees{{region: substitution.graph.unkR}: false}
		}
		base = substitution.graph.pointees(substitution.common.Args[at.Root.Index])
	case HeapGlobal:
		base = substitution.globalSlots(at.Root)
	case HeapResult:
		base = pointees{{region: substitution.freshObject("result:" + strconv.Itoa(at.Root.Index))}: false}
	case HeapFreeVar:
		return pointees{{region: substitution.graph.unkR}: false}
	}
	if at.Path == "" {
		return base
	}
	for _, step := range ssaflow.SplitAccessPath(at.Path) {
		base = substitution.graph.selectStep(base, step)
	}
	return base
}

// globalSlots names the caller's slot for a package variable the callee
// touched, looking the variable up in the program so a global this
// function never mentions still gets its own object; one the program cannot
// find is a foreign object of its own, distinct from everything the caller
// knows, as the structural contract requires.
func (substitution *heapSubstitution) globalSlots(root HeapRoot) pointees {
	if callee := substitution.common.StaticCallee(); callee != nil && callee.Prog != nil {
		if pkg := callee.Prog.ImportedPackage(root.Package); pkg != nil {
			if global, ok := pkg.Members[root.Name].(*ssa.Global); ok {
				return pointees{{region: substitution.graph.external(global)}: false}
			}
		}
	}
	return pointees{{region: substitution.foreignObject("global:" + root.Package + "." + root.Name)}: false}
}

// foreignObject interns an object the function did not allocate and cannot
// otherwise name: a global the program does not expose, or content the
// callee wrote that its summary could not describe.
func (substitution *heapSubstitution) foreignObject(label string) *region {
	value, _ := substitution.instruction.(ssa.Value)
	return substitution.graph.intern(regionKey{kind: regionExternal, origin: value, label: label})
}

// freshObject interns an object the callee created, named by its origin.
func (substitution *heapSubstitution) freshObject(origin string) *region {
	if object, ok := substitution.fresh[origin]; ok {
		return object
	}
	value, _ := substitution.instruction.(ssa.Value)
	object := substitution.graph.intern(regionKey{kind: regionOpaque, origin: value, label: origin})
	substitution.fresh[origin] = object
	return object
}

// targets resolves what a summary target stands for in the caller. A
// target the summary could not describe is not "anything": under the
// structural contract it is content the function did not write, so the
// caller's slot becomes foreign content, and a result becomes one fresh
// object, exactly as an unresolved call would leave them.
func (substitution *heapSubstitution) targets(target HeapTarget, at HeapSlot) (pointees, bool) {
	switch target.Kind {
	case HeapTargetNil:
		return pointees{{region: substitution.graph.nilR}: false}, true
	case HeapTargetUnknown:
		// Content the summary could not describe is, from here, one
		// foreign object per slot: not "anything", which would alias
		// everything the caller knows, but something the caller never
		// named. An undescribed result is the call's own fresh object; an
		// undescribed slot beneath a result is foreign content like any
		// other, never the result itself.
		if at.Root.Kind == HeapResult && at.Path == "" {
			return pointees{{region: substitution.freshObject("result:" + strconv.Itoa(at.Root.Index))}: false}, true
		}
		return pointees{{region: substitution.foreignObject("content:" + at.String())}: false}, true
	case HeapTargetFresh:
		return pointees{{region: substitution.freshObject("fresh:" + target.Origin + "#" + strconv.Itoa(target.Object))}: false}, true
	case HeapTargetSlot:
		if target.Slot.Path == "" {
			return substitution.slots(target.Slot), true
		}
		// Whatever the caller holds at the slot: its content, not the slot.
		result := pointees{}
		for held, stale := range substitution.slots(target.Slot) {
			for pointee, pointeeStale := range substitution.graph.content(substitution.state, held) {
				result.add(pointee, stale || pointeeStale)
			}
		}
		return result, true
	case HeapTargetAddress:
		return substitution.slots(target.Slot), true
	}
	return nil, false
}

// apply writes one edge into the caller's state.
func (substitution *heapSubstitution) apply(edge HeapEdge) {
	value, ok := substitution.targets(edge.To, edge.From)
	if !ok {
		substitution.forget(edge.From)
		return
	}
	if edge.From.Root.Kind == HeapResult {
		if substitution.returned == nil {
			substitution.returned = map[int]pointees{}
		}
		if edge.From.Path == "" {
			set := substitution.returned[edge.From.Root.Index]
			if set == nil {
				set = pointees{}
				substitution.returned[edge.From.Root.Index] = set
			}
			set.union(value)
			return
		}
	}
	destinations := substitution.slots(edge.From)
	single, exact := singleSlot(destinations)
	if edge.Must && exact && lastStep(single.path) != pathStar {
		substitution.graph.clearSubtree(substitution.state, single)
		substitution.graph.forgetWholeAbove(substitution.state, single)
		substitution.state.contents[single] = value.clone()
		substitution.graph.remember(single, value)
		return
	}
	for destination := range destinations {
		if destination.region.kind == regionUnknown || destination.region.kind == regionNil {
			continue
		}
		existing, ok := substitution.state.contents[destination]
		if !ok {
			existing = substitution.graph.content(substitution.state, destination).clone()
			substitution.state.contents[destination] = existing
		}
		existing.union(value)
		substitution.graph.remember(destination, value)
		substitution.graph.bound(substitution.state, destination, substitution.instruction)
	}
}

// truncate forgets everything beneath a slot the summary could not describe.
func (substitution *heapSubstitution) truncate(at HeapSlot) {
	if at.Root.Kind == HeapResult {
		return
	}
	set := substitution.slots(at)
	if set.unknown() {
		substitution.state.opaque = true
		substitution.graph.invalidateForeign(substitution.state, "", substitution.graph.id(substitution.instruction), reachAny)
		return
	}
	substitution.forgetSlots(set)
}

// forget makes the caller's slot foreign content: the callee wrote it with
// something the summary could not name.
func (substitution *heapSubstitution) forget(at HeapSlot) {
	if at.Root.Kind == HeapResult {
		return
	}
	substitution.forgetSlots(substitution.slots(at))
}

func (substitution *heapSubstitution) forgetSlots(set pointees) {
	for target := range set {
		if target.region.kind == regionSite {
			substitution.graph.clearSubtree(substitution.state, target)
			substitution.state.clobbered[target] = substitution.graph.id(substitution.instruction)
			continue
		}
		// A closure the callee may have run wrote through what it captured.
		if target.region.kind == regionClosure {
			substitution.graph.clobber(substitution.state, pointees{target: false}, substitution.graph.id(substitution.instruction))
		}
		// A foreign object the callee may have rewritten beneath: forget
		// what was known there and restamp its unwritten content.
		for known := range substitution.state.contents {
			if known.region == target.region && slotBeneath(known.path, target.path) {
				delete(substitution.state.contents, known)
			}
		}
		substitution.state.stepEpochs[stepKey(target.path)] = substitution.graph.id(substitution.instruction)
	}
}

// results records what the call's results refer to, for the call value or
// the extracts that select them. An object this call created is named by
// the SSA value that holds it, the extract for a multi-result call, so a
// proof that resolves a cell to the object finds the value the caller uses.
func (substitution *heapSubstitution) results(call *ssa.Call) {
	count := call.Common().Signature().Results().Len()
	for index := range count {
		set := substitution.returned[index]
		if set == nil {
			set = pointees{{region: substitution.freshObject("result:" + strconv.Itoa(index))}: false}
		}
		if count == 1 {
			substitution.graph.values[call] = set.clone()
			return
		}
		if extract := ssaflow.CallResult(call, index); extract != nil {
			for pointee := range set {
				if pointee.region.kind == regionOpaque && pointee.region.origin == ssa.Value(call) && pointee.path == "" {
					pointee.region.origin = extract
				}
			}
		}
		if substitution.graph.callResults == nil {
			substitution.graph.callResults = map[*ssa.Call][]pointees{}
		}
		if substitution.graph.callResults[call] == nil {
			substitution.graph.callResults[call] = make([]pointees, count)
		}
		substitution.graph.callResults[call][index] = set.clone()
	}
}
