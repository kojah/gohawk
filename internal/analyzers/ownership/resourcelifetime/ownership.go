package resourcelifetime

import (
	"go/token"
	"go/types"
	"strings"

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
	if !ok || !ssaflow.ValueDerivesFrom(store.Val, resource, map[ssa.Value]bool{}) && !ssaflow.ValueContainsValue(store.Val, resource) {
		return nil
	}
	if field, ok := store.Addr.(*ssa.FieldAddr); ok {
		return field.X
	}
	// A store through a pointer the caller supplied, such as appending to the
	// slice a pointer receiver points at, lands in caller-owned storage.
	// rules_img collects output files through a flag value this way:
	// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/validate/layer-presence/flags.go#L83-L94
	return store.Addr
}

func resourceSuccessBranch(
	pass *analysis.Pass,
	block, successor *ssa.BasicBlock,
	errorValue ssa.Value,
	candidate token.Pos,
) (bool, bool) {
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
	// https://github.com/prometheus/node_exporter/blob/a4e08d1d9a152f67ef781469eade6b0bf431994d/collector/ethtool_linux_test.go#L62-L74
	// https://github.com/pocketbase/pocketbase/blob/bc8ffed4e7265a70a6e8de76c0b0b48b945e19ef/tools/filesystem/internal/fileblob/fileblob.go#L428-L436
	if proof, ok := resourceAbsentErrorCheck(branch.Cond, errorValue); ok && successor == block.Succs[0] {
		traceAcquisitionErrorProof(pass, branch, proof, candidate)
		return false, true
	}
	if success, ok := testifyNoErrorSuccessBranch(branch, successor, errorValue); ok {
		if success {
			traceAcquisitionErrorProof(pass, branch, "testify-no-error-guard", candidate)
		}
		return success, true
	}
	return ssaflow.SuccessBranch(block, successor, errorValue)
}

func testifyNoErrorSuccessBranch(branch *ssa.If, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	call, ok := branch.Cond.(*ssa.Call)
	if !ok || !ssaflow.HasLibraryContract(call.Common(), ssaflow.ContractTestifyNoError) || len(call.Common().Args) < 2 ||
		!ssaflow.SameValue(call.Common().Args[1], errorValue) {
		return false, false
	}
	// Testify's exact boolean contract is true precisely when the supplied
	// error is nil. Go's SSA canonicalizes both `if NoError` and
	// `if !NoError { return }` so the true successor is the success path. Kong
	// uses both forms around HTTP response cleanup:
	// https://github.com/Kong/kong-operator/blob/1adc910f31b5a6bf65d20dbf0698c85f3dbb87b1/ingress-controller/test/helpers/http.go#L173-L177
	return successor == branch.Block().Succs[0], true
}

func resourceAbsentErrorCheck(condition, errorValue ssa.Value) (string, bool) {
	if errorTypeAssertionSucceeded(condition, errorValue) {
		return "error-type-assertion-succeeded", true
	}
	if errorsIsNonNilFilesystemSentinel(condition, errorValue) {
		return "errors-is-non-nil-filesystem-sentinel", true
	}
	call, ok := condition.(*ssa.Call)
	if !ok {
		return "", false
	}
	common := call.Common()
	// os.IsNotExist and os.IsExist are the legacy equivalents of errors.Is with
	// the corresponding filesystem sentinel. Their true branches prove that
	// the acquisition returned a non-nil error and no owned file.
	// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/feedback/send.go#L124
	if len(common.Args) != 1 || !ssaflow.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
		return "", false
	}
	// os.IsPermission and os.IsTimeout are documented to report false for a
	// nil error, so their true branches carry the same proof.
	for _, predicate := range []string{"IsNotExist", "IsExist", "IsPermission", "IsTimeout"} {
		if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("os", predicate)) {
			return "os-" + strings.ToLower(predicate), true
		}
	}
	return "", false
}

func errorTypeAssertionSucceeded(condition, errorValue ssa.Value) bool {
	okResult, ok := condition.(*ssa.Extract)
	if !ok || okResult.Index != 1 {
		return false
	}
	assertion, ok := okResult.Tuple.(*ssa.TypeAssert)
	return ok && assertion.CommaOk && ssaflow.ValueDerivesFrom(assertion.X, errorValue, map[ssa.Value]bool{})
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

func traceAcquisitionErrorProof(pass *analysis.Pass, branch *ssa.If, proof string, candidate token.Pos) {
	analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), candidate).Evidence(analysisTrace.Step{
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
