package concurrencyfacts

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Inert data is ordinary program state that cannot block, synchronize, or
// become a resource identity: a counter, flag, name, or logger reached through
// a caller's pointer. Such storage may change under other goroutines, so an
// inert value never names a channel, primitive, or context. Admitting these
// instructions omits no synchronization effect. Any later use of an inert value
// is still classified on its own, so a dynamic call, type assertion, or
// unmodeled callee keeps stopping the summary. Field addresses and reads can
// panic on a nil owner, which the engine already accepts for field addresses on
// parameters: a path that panics never reaches a later wait.

// inertValue rejects every type that could carry a resource identity directly.
// A value that embeds a primitive is rejected too, since copying it would copy
// lock state.
func inertValue(value types.Type) bool {
	if _, channel := value.Underlying().(*types.Chan); channel {
		return false
	}
	return !containsSynchronization(value) && !synchronizationPointer(value) && !cancellationType(value)
}

func inertDataInstruction(instruction ssa.Instruction) bool {
	switch instruction := instruction.(type) {
	case *ssa.FieldAddr:
		// A primitive's own fields are its internal state, never inert data.
		return !synchronizationPointer(instruction.X.Type()) && inertAddress(instruction)
	case *ssa.Store:
		// Writing a flag or counter cannot satisfy a modeled wait. A child that
		// reads it only chooses among branches the summary already keeps.
		return inertValue(instruction.Val.Type()) && inertAddress(instruction.Addr)
	case *ssa.BinOp:
		// Comparing against nil never panics, even for interfaces.
		return (instruction.Op == token.EQL || instruction.Op == token.NEQ) &&
			(nilConstant(instruction.X) || nilConstant(instruction.Y))
	default:
		return false
	}
}

func inertAddress(address ssa.Value) bool {
	pointer, ok := address.Type().Underlying().(*types.Pointer)
	return ok && inertValue(pointer.Elem())
}

func nilConstant(value ssa.Value) bool {
	constant, ok := value.(*ssa.Const)
	return ok && constant.IsNil()
}
