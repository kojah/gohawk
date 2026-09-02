package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Resource flow tracks an acquired value from the successful call edge to each
// feasible normal return. State records activation and release separately so
// error-only resources and path-specific cleanup do not create false leaks.

type resourceFlowState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	index       int
	active      bool
	released    bool
}

type resourceFlowKey struct {
	block       int
	predecessor int
	index       int
	active      bool
	released    bool
}

// Analyzer returns this package's configured Go analysis pass.

func evaluateResourceFlow(
	pass *analysis.Pass,
	evidence *lifecyclefacts.LifecycleEvidence,
	call *ssa.Call,
	resource ssa.Value,
	contract resourceContract,
) resourceLifetimePolicyResult {
	index := ssaflow.InstructionIndex(call)
	if index < 0 {
		return acceptedResourceLifetime(resourceReasonReleaseProven)
	}
	errorValue := ssaflow.CallResult(call, 1)
	if testProvesAcquisitionError(call, resource, errorValue, contract.packagePath == "net/http") {
		return acceptedResourceLifetime(resourceReasonReleaseProven)
	}
	optionalAcquisition := proveOptionalAcquisition(call, resource, errorValue)
	if optionalAcquisition.Proven() {
		resource = optionalAcquisition.resourcePhi
	}
	owners := localResourceOwners(call.Parent(), resource)
	queue := []resourceFlowState{{block: call.Block(), index: index + 1, active: true}}
	seen := map[resourceFlowKey]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := resourceStateKey(state)
		if seen[key] {
			continue
		}
		seen[key] = true
		var leaks bool
		state, leaks = advanceResourceState(pass, evidence, state, resource, owners, contract, optionalAcquisition)
		if leaks {
			return reportedResourceLifetime(resourceReasonUnownedReturn)
		}
		queue = append(queue, resourceSuccessorStates(pass, state, errorValue, resource, optionalAcquisition)...)
	}
	return acceptedResourceLifetime(resourceReasonReleaseProven)
}

func resourceStateKey(state resourceFlowState) resourceFlowKey {
	predecessor := -1
	if state.predecessor != nil {
		predecessor = state.predecessor.Index
	}
	return resourceFlowKey{
		block:       state.block.Index,
		predecessor: predecessor,
		index:       state.index,
		active:      state.active,
		released:    state.released,
	}
}

func advanceResourceState(
	pass *analysis.Pass,
	evidence *lifecyclefacts.LifecycleEvidence,
	state resourceFlowState,
	resource ssa.Value,
	owners []ssa.Value,
	contract resourceContract,
	optionalAcquisition optionalAcquisitionProof,
) (resourceFlowState, bool) {
	for _, instruction := range state.block.Instrs[state.index:] {
		state.released = state.released ||
			releasesResource(evidence, instruction, resource, owners, contract.cleanup, optionalAcquisition) ||
			contract.consumable && consumesResource(instruction, resource)
		if ssaflow.InstructionTerminatesControlFlow(instruction) {
			state.active = false
			break
		}
		returned, ok := instruction.(*ssa.Return)
		if ok && state.active && !state.released &&
			!returnedResourceOwner(pass, returned, resource, contract.cleanup) &&
			!ssaflow.ReturnedSameAsAny(returned, owners) {
			return state, true
		}
	}
	return state, false
}

func resourceSuccessorStates(
	pass *analysis.Pass,
	state resourceFlowState,
	errorValue, resource ssa.Value,
	optionalAcquisition optionalAcquisitionProof,
) []resourceFlowState {
	successors := ssaflow.FeasibleSuccessors(state.block, state.predecessor)
	if optionalAcquisition.Proven() && state.block == optionalAcquisition.merge && state.predecessor == optionalAcquisition.acquisitionBlock {
		successors = []*ssa.BasicBlock{optionalAcquisition.acquiredSuccessor}
		traceOptionalAcquisition(pass, optionalAcquisition)
	}
	result := make([]resourceFlowState, 0, len(successors))
	for _, successor := range successors {
		active := state.active
		if success, known := resourceSuccessBranch(pass, state.block, successor, errorValue); known {
			active = active && success
		}
		if present, known := resourcePresenceBranch(state.block, successor, resource); known {
			active = active && present
		}
		result = append(result, resourceFlowState{block: successor, predecessor: state.block, active: active, released: state.released})
	}
	return result
}

func returnedResourceOwner(pass *analysis.Pass, returned *ssa.Return, resource ssa.Value, cleanup []string) bool {
	if ssaflow.ReturnedValueOwnsValue(returned, resource) {
		return true
	}
	for _, result := range returned.Results {
		if !ssaflow.ValueDerivesFrom(result, resource, map[ssa.Value]bool{}) {
			continue
		}
		methods := types.NewMethodSet(result.Type())
		for method := range methods.Methods() {
			if slices.Contains(cleanup, method.Obj().Name()) {
				if analysisTrace.Enabled("resourcelifetime", string(check.ResourceRelease)) {
					analysisTrace.Emit(
						pass,
						analysisTrace.Event{
							Analyzer: "resourcelifetime",
							Check:    string(check.ResourceRelease),
							Phase:    "evidence",
							Reason:   "returned-cleanup-projection",
							Outcome:  analysisTrace.OutcomeAccepted,
							Pos:      returned.Pos(),
							Function: returned.Parent().String(),
						},
					)
				}
				return true
			}
		}
	}
	return false
}

func testProvesAcquisitionError(acquisition *ssa.Call, resource, errorValue ssa.Value, httpResponse bool) bool {
	// Test assertions can prove the owned-resource path infeasible even though
	// the assertion package expresses that fact outside the CFG.
	// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/cmd/wfx/cmd/root/root_test.go#L154-L157
	errorAssertions, nilAssertions := httpErrorAssertions(acquisition, resource, errorValue)
	// A fatal Error assertion stops the test unless the acquisition failed,
	// which is the same evidence as an `if err != nil { return }` guard for any
	// acquisition. The non-fatal form is accepted only for net/http, whose
	// paired Nil assertion carries the extra fact that a response returned
	// together with an error has an already-closed body.
	for _, assertedError := range errorAssertions {
		if fatalErrorAssertion(assertedError) || httpResponse && errorAssertionDominatesNil(assertedError, nilAssertions) {
			return true
		}
	}
	return false
}

func httpErrorAssertions(acquisition *ssa.Call, resource, errorValue ssa.Value) ([]ssa.Instruction, []ssa.Instruction) {
	var errorAssertions, nilAssertions []ssa.Instruction
	for _, block := range acquisition.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !ssaflow.InstructionMayFollow(acquisition, instruction) {
				continue
			}
			common := ssaflow.InstructionCall(instruction)
			if !ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyAssertion) {
				continue
			}
			if ssaflow.CallName(common) == "Error" || ssaflow.CallName(common) == "NotNil" {
				for _, argument := range common.Args {
					if ssaflow.ValueDerivesFrom(argument, errorValue, map[ssa.Value]bool{}) {
						errorAssertions = append(errorAssertions, instruction)
					}
				}
			}
			if ssaflow.CallName(common) == "Nil" {
				for _, argument := range common.Args {
					if ssaflow.SameValue(argument, resource) {
						nilAssertions = append(nilAssertions, instruction)
					}
				}
			}
		}
	}
	return errorAssertions, nilAssertions
}

func errorAssertionDominatesNil(assertedError ssa.Instruction, nilAssertions []ssa.Instruction) bool {
	for _, assertedNil := range nilAssertions {
		if ssaflow.InstructionDominates(assertedError, assertedNil) {
			return true
		}
	}
	return false
}

func fatalErrorAssertion(instruction ssa.Instruction) bool {
	common := ssaflow.InstructionCall(instruction)
	return ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyFatalError)
}

func resourcePresenceBranch(block, successor *ssa.BasicBlock, resource ssa.Value) (bool, bool) {
	if resource == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	comparesResourceToNil := ssaflow.ValueDerivesFrom(comparison.X, resource, map[ssa.Value]bool{}) && ssaflow.DefinitelyNil(comparison.Y) ||
		ssaflow.ValueDerivesFrom(comparison.Y, resource, map[ssa.Value]bool{}) && ssaflow.DefinitelyNil(comparison.X)
	if !comparesResourceToNil {
		return false, false
	}
	trueBranch := successor == block.Succs[0]
	// On the nil branch there is no owned value to release. This matters when
	// callers defensively close a response whenever net/http returns one, even
	// on an error path:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/codex_utls_transport_test.go#L305-L319
	if comparison.Op == token.NEQ {
		return trueBranch, true
	}
	return !trueBranch, true
}
