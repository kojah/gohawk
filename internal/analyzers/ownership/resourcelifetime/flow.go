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
	// unknown records that something the analysis cannot see through
	// consumed the resource on this path; a return after that proves nothing.
	unknown bool
}

type resourceFlowKey struct {
	block       int
	predecessor int
	index       int
	active      bool
	released    bool
	unknown     bool
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
	if deferredBeforeAcquisitionMayRelease(evidence, call, resource, contract.cleanup) {
		return acceptedResourceLifetime(resourceReasonReleaseProven)
	}
	owners := localResourceOwners(call.Parent(), resource)
	analysis := &resourceAnalysis{
		pass: pass, evidence: evidence, function: call.Parent(), resource: resource, owners: owners,
		contract: contract, optional: optionalAcquisition, actions: map[ssa.Instruction]resourceAction{},
	}
	// The walk starts on the instruction after the acquisition and keys its
	// states by block, predecessor, and release status, so the same block is
	// revisited only when a different path reaches it with a different
	// obligation state; the predecessor lets the successful branch of the
	// acquisition be told apart from its error branch.
	queue := []resourceFlowState{{block: call.Block(), index: index + 1, active: true}}
	seen := map[resourceFlowKey]bool{}
	opaque := false
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := resourceStateKey(state)
		if seen[key] {
			continue
		}
		seen[key] = true
		var leaks bool
		state, leaks = advanceResourceState(pass, analysis, state)
		if leaks {
			return reportedResourceLifetime(resourceReasonUnownedReturn)
		}
		opaque = opaque || state.unknown
		queue = append(queue, resourceSuccessorStates(pass, state, errorValue, resource, optionalAcquisition)...)
	}
	if opaque {
		return acceptedResourceLifetime(resourceReasonOpaqueConsumption)
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
		unknown:     state.unknown,
	}
}

func advanceResourceState(pass *analysis.Pass, analysis *resourceAnalysis, state resourceFlowState) (resourceFlowState, bool) {
	// A release or transfer anywhere before a return settles the path. An
	// opaque consumption does not settle it but removes the proof: the
	// return is then neither owned nor a defect.
	for _, instruction := range state.block.Instrs[state.index:] {
		switch analysis.action(instruction) {
		case actionSettled:
			state.released = true
		case actionUnknown:
			state.unknown = true
		case actionNone:
		}
		if ssaflow.InstructionTerminatesControlFlow(instruction) {
			state.active = false
			break
		}
		returned, ok := instruction.(*ssa.Return)
		if ok && state.active && !state.released && !state.unknown &&
			!returnedResourceOwner(pass, returned, analysis.resource, analysis.contract.cleanup) &&
			!ssaflow.ReturnedSameAsAny(returned, analysis.owners) {
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
		// A returned view is summarized as releasing nothing, whatever its
		// method names suggest; the caller of this function cannot close the
		// resource through it.
		if call, ok := result.(*ssa.Call); ok && lifecyclefacts.CallReturnsView(pass, call, resource) {
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

// deferredBeforeAcquisitionMayRelease reports whether a defer registered on
// every path to the acquisition may release the resource: typically a literal
// that drains a captured closer slice the resource is appended to later. The
// walk below only classifies instructions after the acquisition, so such a
// defer is asked here, with may-release coverage because the deferred
// literal decides at return time how many entries it closes. rules_img opens
// inputs into a closer slice under one deferred drain loop:
// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/mtree/mtree.go#L110-L128
func deferredBeforeAcquisitionMayRelease(
	evidence *lifecyclefacts.LifecycleEvidence,
	call *ssa.Call,
	resource ssa.Value,
	methods []string,
) bool {
	for _, block := range call.Parent().Blocks {
		for _, instruction := range block.Instrs {
			deferred, ok := instruction.(*ssa.Defer)
			if !ok || !ssaflow.InstructionDominates(deferred, call) {
				continue
			}
			completion := ssaflow.CompletionRequest{
				Instruction: deferred,
				Target:      resource,
				Methods:     methods,
				Coverage:    ssaflow.CoverageAnywhere,
			}
			if evidence.Prove(lifecyclefacts.EvidenceRequest{Instruction: deferred, Target: resource, Completion: &completion}).Proven() {
				return true
			}
		}
	}
	return false
}
