package resourcelifetime

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
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
		Budget:      analysis.budget(releaseSearchBudget),
	}).Proven()
}

// A helper may release every element of the aggregate it receives inside a
// loop, as slackdump's Destroy closes each stored handle:
// https://github.com/rusq/slackdump/blob/f7319928b0993b23d7e9bd8af5e4c69b6f1d2af4/internal/chunk/filemgr.go#L66-L73
// The every-return proof cannot credit a call inside a cycle: the loop's exit
// edge is a path that skips the body, and which element an iteration releases
// is decided by iteration rather than by the path. That is uncertainty about
// the element, not evidence of a leak, so the call is opaque. A helper whose
// cleanup merely depends on a flag has complete path information and stays
// diagnostic; so does one that loops over some other collection. A callee in
// another package has no body here; its summary carries the same loop as a
// may-claim, which client-go's CloseAndRemove exports for its variadic files.
func (analysis *resourceAnalysis) loopedHelperCleanup(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	if len(callee.Blocks) == 0 {
		for index, argument := range common.Args {
			if released, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimReleasesInLoop); released &&
				analysis.carries(argument) {
				return true
			}
		}
		return false
	}
	for _, binding := range ssaflow.CallBindings(common, callee, nil) {
		if !analysis.carries(binding.Supplied) {
			continue
		}
		for _, block := range callee.Blocks {
			if analysis.blockReleasesLocal(block, binding.Local) && ssaflow.BlockInCycle(block) {
				return true
			}
		}
	}
	return false
}

// blockReleasesLocal reports whether the block calls one of the contract's
// cleanup methods on a value derived from the callee local.
func (analysis *resourceAnalysis) blockReleasesLocal(block *ssa.BasicBlock, local ssa.Value) bool {
	for _, candidate := range block.Instrs {
		call := ssaflow.InstructionCall(candidate)
		if call != nil && slices.Contains(analysis.contract.cleanup, ssaflow.CallName(call)) &&
			ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(call), local, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
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
