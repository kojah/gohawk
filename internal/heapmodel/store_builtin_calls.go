package heapmodel

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Synchronous builtin effects operate on the selected storage. Deferred calls
// use this same implementation at execution; asynchronous calls stay opaque.
// https://go.dev/ref/spec#Appending_to_and_copying_slices
// https://go.dev/ref/spec#Clear

// builtin applies the builtins that touch memory. Append may share the
// backing array with its first argument and stores the rest into it; copy
// stores the source's elements into the destination.
func (graph *regionGraph) builtin(state *regionState, builtin *ssa.Builtin, common *ssa.CallCommon, instruction ssa.Instruction) {
	switch builtin.Name() {
	case "append":
		value, _ := instruction.(ssa.Value)
		if len(common.Args) == 0 || value == nil {
			return
		}
		result := graph.pointees(common.Args[0]).clone()
		result.add(slot{region: graph.opaque(value)}, false)
		graph.setValue(value, result)
		for _, argument := range common.Args[1:] {
			elements := graph.pointees(argument)
			if _, ok := argument.Type().Underlying().(*types.Slice); ok {
				// The spread slice's elements are copied into a collection
				// the function may hand on, so the array behind it has
				// escaped as far as its contents are concerned.
				graph.escape(state, elements, HeapEscapedField, instruction)
				elements = graph.load(state, graph.selectStep(elements, pathStar), argument)
			}
			for target := range result {
				graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, pathStar)}, elements)
			}
			graph.escape(state, elements, HeapEscapedField, instruction)
		}
	case "copy":
		if len(common.Args) < 2 {
			return
		}
		elements := graph.load(state, graph.selectStep(graph.pointees(common.Args[1]), pathStar), common.Args[1])
		for target := range graph.pointees(common.Args[0]) {
			graph.weakElementStore(state, slot{region: target.region, path: joinSlotPath(target.path, pathStar)}, elements)
		}
	case "clear":
		if len(common.Args) != 1 {
			return
		}
		// Clearing writes the collection's elements, not the objects those
		// elements previously pointed to. Forget only the selected storage;
		// do not expose it or recursively clobber the former pointees. We do
		// not infer exact zero contents or the extent of an arbitrary view.
		substitution := heapSubstitution{graph: graph, state: state, instruction: instruction}
		substitution.forgetSlots(graph.pointees(common.Args[0]))
	case "delete", "len", "cap", "print", "println", "close", "recover", "min", "max", "panic", "real", "imag", "complex", "new":
	}
}
