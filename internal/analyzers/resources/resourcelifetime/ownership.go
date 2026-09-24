package resourcelifetime

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssainfer"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

func localResourceOwners(function *ssa.Function, resource ssa.Value) []ssa.Value {
	var owners []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			owner := resourceFieldOwner(instruction, resource)
			if owner != nil && !ssaflow.ExternallyOwnedValue(owner) && !ssainfer.MayAliasAny(owner, owners) {
				owners = append(owners, owner)
			}
		}
	}
	return owners
}

// A helper can condition cleanup on an error it receives beside the
// resource. Unconditional completion cannot represent that relation, so a
// witnessed cleanup plus a correlated error is uncertainty, not proof of
// either release or a leak. The error is correlated when it is the one
// paired with this acquisition, or when the caller itself branches on it
// being nil after the call: then the caller's own paths split on the same
// value the helper's cleanup does, as in closeOnError(f, err) followed by
// if err != nil { return nil, err }; return f, nil.
// https://github.com/h44z/wg-portal/blob/eb44c8c4ff120f34c26b2415c47560f4fba0603c/internal/lowlevel/mikrotik.go#L267-L280
// Helpers that merely inspect the pair, receive an error the caller never
// tests again, or condition cleanup on a flag stay visible: a flag the
// caller does not branch on leaves the unreleased path feasible.
func (analysis *resourceAnalysis) pairedErrorHelperCleanup(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || !slices.Contains(common.Args, analysis.resource) ||
		!slices.ContainsFunc(common.Args, func(argument ssa.Value) bool { return analysis.correlatedError(instruction, argument) }) {
		return false
	}
	return ssainfer.ProveCompletion(ssainfer.CompletionRequest{
		Instruction: instruction,
		Target:      analysis.resource,
		Methods:     analysis.contract.cleanup,
		Coverage:    ssainfer.CoverageAnywhere,
		Budget:      analysis.budget(releaseSearchBudget),
	}).Proven()
}

// correlatedError reports whether an error handed to the helper call is the
// acquisition's paired error, or one the caller compares with nil after the
// call. Identity, not derivation: a wrapped error is a different value.
func (analysis *resourceAnalysis) correlatedError(call ssa.Instruction, argument ssa.Value) bool {
	if !syntax.IsErrorType(argument.Type()) {
		return false
	}
	if analysis.resource == ssaflow.CallResult(analysis.acquisition, 0) && argument == ssaflow.CallResult(analysis.acquisition, 1) {
		return true
	}
	for _, instruction := range ssaflow.InstructionsReachableAfter(call) {
		branch, ok := instruction.(*ssa.If)
		if !ok {
			continue
		}
		comparison, ok := branch.Cond.(*ssa.BinOp)
		if ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) &&
			(comparison.X == argument && ssaflow.DefinitelyNil(comparison.Y) || comparison.Y == argument && ssaflow.DefinitelyNil(comparison.X)) {
			return true
		}
	}
	return false
}

// An imported helper that releases every element of what it receives inside
// a loop exports that loop as a may-claim, as client-go's CloseAndRemove does
// for its variadic files; a visible helper's loop is found by the completion
// search itself. Either way the call is uncertain, never a release.
// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/staging/src/k8s.io/client-go/util/testing/remove_file.go#L25-L39
func (analysis *resourceAnalysis) importedLoopRelease(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) != 0 {
		return false
	}
	for index, argument := range common.Args {
		if released, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimReleasesInLoop); released &&
			analysis.carries(argument) {
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
	if !ok || !ssainfer.ValueDerivesFrom(store.Val, resource, map[ssa.Value]bool{}) && !ssainfer.MayContainValue(store.Val, resource) {
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
