package ssaflow

import "golang.org/x/tools/go/ssa"

// A channel made in a function is used through several SSA values: the make
// itself, loads of a variable cell written once with it, which is how a
// closure captures it, a closure's loads of its copy of that cell, direction
// conversions, and the parameter of a named function it is passed to. These
// helpers name every such value and every other use, so a caller can decide
// whether it has seen all the channel's operations. What counts as a
// complete census is the caller's policy.

// ChannelUse is one instruction that uses a value of the channel other than
// to move it between the values ChannelValues follows.
type ChannelUse struct {
	Value       ssa.Value
	Instruction ssa.Instruction
}

// ChannelValues returns the values that are the channel made by made within
// its function and within the static callees and closures it is passed to
// or captured by, and every use of those values that is not one of the moves
// followed: a store into a written-once cell, a load of it, a closure
// binding of it, a direction conversion, or a static call argument. A call
// that passes the channel to a callee without a body is returned as a use.
func ChannelValues(made *ssa.MakeChan) ([]ssa.Value, []ChannelUse) {
	values := []ssa.Value{made}
	member := map[ssa.Value]bool{made: true}
	pending := []ssa.Value{made}
	var uses []ChannelUse
	for len(pending) != 0 {
		value := pending[0]
		pending = pending[1:]
		for _, user := range *value.Referrers() {
			moved := channelMove(value, user)
			if moved == nil {
				uses = append(uses, ChannelUse{Value: value, Instruction: user})
				continue
			}
			for _, target := range moved {
				if target != nil && !member[target] {
					member[target] = true
					values = append(values, target)
					pending = append(pending, target)
				}
			}
		}
	}
	return values, uses
}

// channelMove returns the values a channel value becomes through user, or
// nil when user is a use of the channel rather than a move.
func channelMove(value ssa.Value, user ssa.Instruction) []ssa.Value {
	switch typed := user.(type) {
	case *ssa.ChangeType:
		return []ssa.Value{typed}
	case *ssa.Store:
		cell, ok := typed.Addr.(*ssa.Alloc)
		if !ok || typed.Val != value {
			return nil
		}
		if stored, once := WrittenOnceCell(cell); !once || stored != value {
			return nil
		}
		return cellCopies(cell)
	case *ssa.UnOp:
		return nil
	case *ssa.Call, *ssa.Go, *ssa.Defer:
		return argumentParameters(value, InstructionCall(user))
	}
	return nil
}

// cellCopies returns the loads of a cell and of every closure's captured
// copy of it, provided the cell is used only by stores, loads, and closure
// bindings.
func cellCopies(cell *ssa.Alloc) []ssa.Value {
	var copies []ssa.Value
	for _, user := range *cell.Referrers() {
		switch typed := user.(type) {
		case *ssa.Store:
		case *ssa.UnOp:
			copies = append(copies, typed)
		case *ssa.MakeClosure:
			for index, binding := range typed.Bindings {
				if binding != cell {
					continue
				}
				captured := typed.Fn.(*ssa.Function).FreeVars[index]
				for _, load := range *captured.Referrers() {
					if unop, ok := load.(*ssa.UnOp); ok {
						copies = append(copies, unop)
					} else {
						return nil
					}
				}
			}
		default:
			return nil
		}
	}
	return copies
}

// argumentParameters returns the parameters of a static callee with a body
// that receive value, or nil when the call is not such a call.
func argumentParameters(value ssa.Value, common *ssa.CallCommon) []ssa.Value {
	if common == nil || common.IsInvoke() {
		return nil
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 || len(callee.Params) != len(common.Args) {
		return nil
	}
	var parameters []ssa.Value
	for index, argument := range common.Args {
		if argument == value {
			parameters = append(parameters, callee.Params[index])
		}
	}
	return parameters
}
