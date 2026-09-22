package resourcelifetime

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

func localResourceOwners(function *ssa.Function, resource ssa.Value) []ssa.Value {
	var owners []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			owner := resourceFieldOwner(instruction, resource)
			if owner != nil && !ssaflow.ExternallyOwnedValue(owner) && !ssaflow.MayAliasAny(owner, owners) {
				owners = append(owners, owner)
			}
		}
	}
	return owners
}

// A helper can condition cleanup on the error paired with this acquisition.
// Unconditional completion cannot represent that relation. A witnessed cleanup
// plus the exact pair is uncertainty, not proof of either release or a leak.
// Helpers that merely inspect the pair, or receive another error, stay visible.
// https://github.com/h44z/wg-portal/blob/eb44c8c4ff120f34c26b2415c47560f4fba0603c/internal/lowlevel/mikrotik.go#L267-L280
func (analysis *resourceAnalysis) pairedErrorHelperCleanup(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || analysis.resource != ssaflow.CallResult(analysis.acquisition, 0) {
		return false
	}
	errorValue := ssaflow.CallResult(analysis.acquisition, 1)
	if errorValue == nil || !syntax.IsErrorType(errorValue.Type()) ||
		!slices.Contains(common.Args, analysis.resource) || !slices.Contains(common.Args, errorValue) {
		return false
	}
	return ssaflow.ProveCompletion(ssaflow.CompletionRequest{
		Instruction: instruction,
		Target:      analysis.resource,
		Methods:     analysis.contract.cleanup,
		Coverage:    ssaflow.CoverageAnywhere,
		Budget:      ssaflow.NewSearchBudget(releaseSearchBudget),
	}).Proven()
}

func resourceTransferredToExternalField(instruction ssa.Instruction, resource ssa.Value) bool {
	owner := resourceFieldOwner(instruction, resource)
	return owner != nil && ssaflow.ExternallyOwnedValue(owner)
}

func resourceFieldOwner(instruction ssa.Instruction, resource ssa.Value) ssa.Value { //nolint:ireturn // Owners retain their concrete SSA value forms.
	store, ok := instruction.(*ssa.Store)
	if !ok || !ssaflow.ValueDerivesFrom(store.Val, resource, map[ssa.Value]bool{}) && !ssaflow.MayContainValue(store.Val, resource) {
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

func resourceLifecycleMethod(name string) bool {
	switch name {
	case "Close", "Kill", "Shutdown", "Stop", "Wait":
		return true
	default:
		return false
	}
}
