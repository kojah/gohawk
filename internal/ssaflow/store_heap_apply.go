package ssaflow

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
	if callee == nil || common.IsInvoke() {
		return false
	}
	summary, ok := heapSummaryOf(callee)
	if !ok || len(callee.FreeVars) != 0 {
		return false
	}
	call, isCall := instruction.(*ssa.Call)
	substitution := &heapSubstitution{graph: graph, state: state, common: common, instruction: instruction, fresh: map[string]*region{}}
	for _, at := range summary.Truncated {
		substitution.truncate(at)
	}
	for _, edge := range summary.Edges {
		substitution.apply(edge)
	}
	for _, effect := range summary.Effects {
		if effect.Escape != 0 {
			graph.escape(state, substitution.slots(effect.Slot), effect.Escape)
		}
	}
	if isCall {
		substitution.results(call)
	}
	return true
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
	for _, step := range SplitAccessPath(at.Path) {
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
		// named.
		if at.Root.Kind == HeapResult {
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
		substitution.graph.invalidateForeign(substitution.state, "", substitution.graph.id(substitution.instruction))
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
		if extract := CallResult(call, index); extract != nil {
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
