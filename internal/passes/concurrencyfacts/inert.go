package concurrencyfacts

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Inert data is ordinary program state that cannot block, synchronize, or
// become a resource identity: a counter, flag, name, logger, or map of such
// values, including one reached through a caller's pointer. Such storage may change under other goroutines, so an
// inert value never names a channel, primitive, or context. Admitting these
// instructions omits no synchronization effect. Any later use of an inert value
// is still classified on its own, so a dynamic call, type assertion, or
// unmodeled callee keeps stopping the summary. Field addresses, reads, and
// container operations can panic on a nil owner, a bad index, or a nil map,
// which the engine already accepts for field addresses on parameters: a path
// that panics never reaches a later wait.

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
	case *ssa.MakeMap, *ssa.MakeSlice:
		// A new empty container holds no resource. A bad size can panic, and a
		// path that panics never reaches a later wait.
		return true
	case *ssa.IndexAddr:
		return inertAddress(instruction)
	case *ssa.Slice:
		// Reslicing shares elements; it creates no new identity.
		return inertElements(instruction.Type())
	case *ssa.Lookup:
		// Reading a map or string element yields that element, or with CommaOk
		// the element and a Boolean.
		return inertValue(lookupElement(instruction))
	case *ssa.TypeAssert:
		// A failed assertion panics, which ends the path.
		return inertValue(lookupElement(instruction))
	case *ssa.Extract:
		// Projecting a result its producer already accounted for. Select
		// results feed the dispatch proof, which keeps its own handling.
		_, selected := instruction.Tuple.(*ssa.Select)
		return !selected && inertValue(instruction.Type())
	case *ssa.MapUpdate:
		// Storing a resource in a map would publish it, so only inert keys and
		// values are admitted.
		return inertValue(instruction.Key.Type()) && inertValue(instruction.Value.Type())
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

// lookupElement returns the value a map lookup or type assertion produces,
// ignoring the CommaOk Boolean.
func lookupElement(value ssa.Value) types.Type {
	if tuple, ok := value.Type().(*types.Tuple); ok {
		return tuple.At(0).Type()
	}
	return value.Type()
}

// Builtins that read lengths, compare scalars, or copy inert elements cannot
// block, synchronize, or publish a resource.
func inertBuiltin(common *ssa.CallCommon) bool {
	builtin, ok := common.Value.(*ssa.Builtin)
	if !ok {
		return false
	}
	switch builtin.Name() {
	case "len", "cap", "min", "max":
		return true
	case "append", "copy":
		return inertElements(common.Args[0].Type()) && (len(common.Args) < 2 || inertElements(common.Args[1].Type()))
	case "delete":
		return inertValue(common.Args[1].Type())
	default:
		return false
	}
}

func inertElements(value types.Type) bool {
	if slice, ok := value.Underlying().(*types.Slice); ok {
		return inertValue(slice.Elem())
	}
	return inertValue(value)
}
