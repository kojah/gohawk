package lifecycle

import (
	"go/token"
	"go/types"
	"strconv"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Test-only feasibility spike, deliberately not an analyzer dependency. Model
// straight-line local storage with exact addresses and strong updates. Anything
// that could mutate unmodeled storage stops the query, rather than guessing.
type smokeLocation struct {
	root *ssa.Alloc
	path string
}

type smokeValue struct {
	leaf     ssa.Value
	location smokeLocation
}

type smokeHeap struct {
	storage map[smokeLocation]smokeValue
	values  map[ssa.Value]smokeValue
}

func smokeHeapIdentity(observation *ssa.Call, budget int) (bool, string) {
	args := observation.Common().Args
	return smokeHeapMatch(observation, args[0], args[1], budget)
}

func smokeHeapMatch(observation ssa.Instruction, leftValue, rightValue ssa.Value, budget int) (bool, string) {
	fn := observation.Parent()
	if len(fn.Blocks) != 1 {
		return false, "control-flow"
	}
	heap := smokeHeap{storage: make(map[smokeLocation]smokeValue), values: make(map[ssa.Value]smokeValue)}
	for _, parameter := range fn.Params {
		heap.values[parameter] = smokeValue{leaf: parameter}
	}
	for _, instruction := range fn.Blocks[0].Instrs {
		budget--
		if budget < 0 {
			return false, "budget"
		}
		if instruction == observation {
			left, leftOK := heap.values[leftValue]
			right, rightOK := heap.values[rightValue]
			return leftOK && rightOK && left == right, "observed"
		}
		if !heap.step(instruction) {
			return false, "unsupported-effect"
		}
	}
	return false, "no-observation"
}

func (heap *smokeHeap) step(instruction ssa.Instruction) bool {
	switch instruction := instruction.(type) {
	case *ssa.Alloc:
		heap.values[instruction] = smokeValue{location: smokeLocation{root: instruction}}
	case *ssa.FieldAddr:
		return heap.project(instruction, instruction.X, ".f"+strconv.Itoa(instruction.Field))
	case *ssa.IndexAddr:
		index, ok := ssaflow.ConstantIndex(instruction.Index)
		return ok && heap.project(instruction, instruction.X, ".i"+index)
	case *ssa.Store:
		switch instruction.Val.Type().Underlying().(type) {
		case *types.Struct, *types.Array:
			return false
		}
		address, ok := heap.values[instruction.Addr]
		value, known := heap.values[instruction.Val]
		if !ok || address.location.root == nil || !known {
			return false
		}
		// Only tracked scalar values and addresses enter storage. Aggregate
		// replacement is unsupported, so overlapping parent/child writes cannot
		// leave stale field entries in this deliberately small model.
		heap.storage[address.location] = value
	case *ssa.UnOp:
		address, ok := heap.values[instruction.X]
		if instruction.Op != token.MUL || !ok || address.location.root == nil {
			return false
		}
		value, known := heap.storage[address.location]
		if !known {
			return false
		}
		heap.values[instruction] = value
	default:
		// Calls, goroutines, closure capture, slices, maps, and aggregate
		// copying require effects this smoke test does not model.
		return false
	}
	return true
}

func (heap *smokeHeap) project(value, base ssa.Value, component string) bool {
	address, ok := heap.values[base]
	if !ok || address.location.root == nil {
		return false
	}
	address.location.path += component
	heap.values[value] = address
	return true
}
