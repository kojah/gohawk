package resourcelifetime

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func localResourceOwners(function *ssa.Function, resource ssa.Value) []ssa.Value {
	var owners []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			owner := resourceFieldOwner(instruction, resource)
			if owner != nil && !ssaflow.ExternallyOwnedValue(owner) && !ssaflow.SameAsAny(owner, owners) {
				owners = append(owners, owner)
			}
		}
	}
	return owners
}

func resourceTransferredToExternalField(instruction ssa.Instruction, resource ssa.Value) bool {
	owner := resourceFieldOwner(instruction, resource)
	return owner != nil && ssaflow.ExternallyOwnedValue(owner)
}

func resourceFieldOwner(instruction ssa.Instruction, resource ssa.Value) ssa.Value { //nolint:ireturn // Owners retain their concrete SSA value forms.
	store, ok := instruction.(*ssa.Store)
	if !ok || !ssaflow.ValueDerivesFrom(store.Val, resource, map[ssa.Value]bool{}) {
		return nil
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	return field.X
}

func resourceSuccessBranch(pass *analysis.Pass, block, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	// A true check against a documented non-nil filesystem error proves that
	// the acquisition failed and produced no owned file. Callers commonly
	// inspect a specific error before the generic err != nil check. This covers
	// both skipped missing inputs and create-if-absent races without treating an
	// arbitrary package variable as non-nil.
	// https://github.com/heymaikol/network-doctor/blob/6d0df6eaba1de237077e0a1f8224fd8d5c3d083a/internal/simulation/evidence.go#L407-L415
	// https://github.com/codefly-dev/cli/blob/5d176b95c8e3ad721bdeb0d6c4c3a64dd261caa6/pkg/executionattestor/file.go#L122-L130
	if proof, ok := resourceAbsentErrorCheck(branch.Cond, errorValue); ok && successor == block.Succs[0] {
		traceAcquisitionErrorProof(pass, branch, proof)
		return false, true
	}
	return ssaflow.SuccessBranch(block, successor, errorValue)
}

func resourceAbsentErrorCheck(condition, errorValue ssa.Value) (string, bool) {
	if errorsIsNonNilFilesystemSentinel(condition, errorValue) {
		return "errors-is-non-nil-filesystem-sentinel", true
	}
	call, ok := condition.(*ssa.Call)
	if !ok {
		return "", false
	}
	common := call.Common()
	// os.IsNotExist is the legacy equivalent of errors.Is(err,
	// fs.ErrNotExist); on its true branch os.Open did not return an owned file.
	// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/feedback/send.go#L124
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("os", "IsNotExist")) && len(common.Args) == 1 &&
		ssaflow.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
		return "os-is-not-exist", true
	}
	return "", false
}

func errorsIsNonNilFilesystemSentinel(condition, errorValue ssa.Value) bool {
	call, ok := condition.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	if !ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("errors", "Is")) || len(common.Args) != 2 {
		return false
	}
	if !ssaflow.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
		return false
	}
	return isNonNilFilesystemSentinel(common.Args[1])
}

func isNonNilFilesystemSentinel(value ssa.Value) bool {
	for {
		if inner, ok := ssaflow.UnwrapTransparentValue(
			value,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		); ok {
			value = inner
			continue
		}
		switch typed := value.(type) {
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return false
			}
			value = typed.X
		case *ssa.Global:
			return ssaflow.ValueMatchesAnySymbol(
				typed,
				syntax.PackageVariable("os", "ErrNotExist"),
				syntax.PackageVariable("os", "ErrExist"),
				syntax.PackageVariable("io/fs", "ErrNotExist"),
				syntax.PackageVariable("io/fs", "ErrExist"),
			)
		default:
			return false
		}
	}
}

func traceAcquisitionErrorProof(pass *analysis.Pass, branch *ssa.If, proof string) {
	if !analysisTrace.Enabled("resourcelifetime", string(check.ResourceRelease)) {
		return
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "resourcelifetime",
		Check:    string(check.ResourceRelease),
		Phase:    "evidence",
		Reason:   "acquisition-error-proven",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      branch.Cond.Pos(),
		Function: branch.Parent().String(),
		Details:  map[string]string{"proof": proof},
	})
}

func consumesResource(instruction ssa.Instruction, resource ssa.Value) bool {
	if receive, ok := instruction.(*ssa.UnOp); ok {
		return receive.Op == token.ARROW && ssaflow.ValueDerivesFrom(receive.X, resource, map[ssa.Value]bool{})
	}
	selection, ok := instruction.(*ssa.Select)
	if !ok {
		return false
	}
	for _, state := range selection.States {
		if state.Dir == types.RecvOnly && ssaflow.ValueDerivesFrom(state.Chan, resource, map[ssa.Value]bool{}) {
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
