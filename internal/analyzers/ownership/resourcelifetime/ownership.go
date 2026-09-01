package resourcelifetime

import (
	"go/token"
	"go/types"

	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"

	"golang.org/x/tools/go/ssa"
)

func localResourceOwners(function *ssa.Function, resource ssa.Value) []ssa.Value {
	var owners []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			owner := resourceFieldOwner(instruction, resource)
			if owner != nil && !ssautil.ExternallyOwnedValue(owner) && !ssautil.SameAsAny(owner, owners) {
				owners = append(owners, owner)
			}
		}
	}
	return owners
}

func resourceTransferredToExternalField(instruction ssa.Instruction, resource ssa.Value) bool {
	owner := resourceFieldOwner(instruction, resource)
	return owner != nil && ssautil.ExternallyOwnedValue(owner)
}

func resourceFieldOwner(instruction ssa.Instruction, resource ssa.Value) ssa.Value { //nolint:ireturn // Owners retain their concrete SSA value forms.
	store, ok := instruction.(*ssa.Store)
	if !ok || !ssautil.ValueDerivesFrom(store.Val, resource, map[ssa.Value]bool{}) {
		return nil
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	return field.X
}

func resourceSuccessBranch(block, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	// A true errors.Is check against the documented non-nil missing-file
	// sentinel proves that os.Open did not produce an owned file. This commonly
	// appears before the generic err != nil check when callers skip missing
	// inputs. Do not carry a live resource around that continue edge.
	// https://github.com/heymaikol/network-doctor/blob/6d0df6eaba1de237077e0a1f8224fd8d5c3d083a/internal/simulation/evidence.go#L407-L415
	if missingFileCheck(branch.Cond, errorValue) && successor == block.Succs[0] {
		return false, true
	}
	return ssautil.SuccessBranch(block, successor, errorValue)
}

func missingFileCheck(condition, errorValue ssa.Value) bool {
	if errorsIsMissingFile(condition, errorValue) {
		return true
	}
	call, ok := condition.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	// os.IsNotExist is the legacy equivalent of errors.Is(err,
	// fs.ErrNotExist); on its true branch os.Open did not return an owned file.
	// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/feedback/send.go#L124
	return ssautil.CallPackage(common) == "os" && ssautil.CallName(common) == "IsNotExist" && len(common.Args) == 1 &&
		ssautil.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{})
}

func errorsIsMissingFile(condition, errorValue ssa.Value) bool {
	call, ok := condition.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	if ssautil.CallPackage(common) != "errors" || ssautil.CallName(common) != "Is" || len(common.Args) != 2 {
		return false
	}
	if !ssautil.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
		return false
	}
	return isMissingFileSentinel(common.Args[1])
}

func isMissingFileSentinel(value ssa.Value) bool {
	for {
		switch typed := value.(type) {
		case *ssa.ChangeInterface:
			value = typed.X
		case *ssa.ChangeType:
			value = typed.X
		case *ssa.Convert:
			value = typed.X
		case *ssa.MakeInterface:
			value = typed.X
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return false
			}
			value = typed.X
		case *ssa.Global:
			if typed.Pkg == nil || typed.Pkg.Pkg == nil || typed.Name() != "ErrNotExist" {
				return false
			}
			packagePath := typed.Pkg.Pkg.Path()
			return packagePath == "os" || packagePath == "io/fs"
		default:
			return false
		}
	}
}

func consumesResource(instruction ssa.Instruction, resource ssa.Value) bool {
	if receive, ok := instruction.(*ssa.UnOp); ok {
		return receive.Op == token.ARROW && ssautil.ValueDerivesFrom(receive.X, resource, map[ssa.Value]bool{})
	}
	selection, ok := instruction.(*ssa.Select)
	if !ok {
		return false
	}
	for _, state := range selection.States {
		if state.Dir == types.RecvOnly && ssautil.ValueDerivesFrom(state.Chan, resource, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func resourceLifecycleMethod(name string) bool {
	switch name {
	case "Close", "Kill", "Shutdown", "Stop", "Wait":
		return true
	default:
		return false
	}
}
