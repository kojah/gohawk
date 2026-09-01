package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

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

func resourceLeaks(pass *analysis.Pass, call *ssa.Call, resource ssa.Value, contract resourceContract) bool {
	index := ssautil.InstructionIndex(call)
	if index < 0 {
		return false
	}
	errorValue := ssautil.CallResult(call, 1)
	if contract.packagePath == "net/http" && testProvesHTTPError(call, resource, errorValue) {
		return false
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
		state, leaks = advanceResourceState(pass, state, resource, owners, contract)
		if leaks {
			return true
		}
		queue = append(queue, resourceSuccessorStates(state, errorValue, resource)...)
	}
	return false
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
	state resourceFlowState,
	resource ssa.Value,
	owners []ssa.Value,
	contract resourceContract,
) (resourceFlowState, bool) {
	for _, instruction := range state.block.Instrs[state.index:] {
		state.released = state.released ||
			releasesResource(pass, instruction, resource, owners, contract.cleanup) ||
			contract.consumable && consumesResource(instruction, resource)
		if ssautil.InstructionTerminatesControlFlow(instruction) {
			state.active = false
			break
		}
		returned, ok := instruction.(*ssa.Return)
		if ok && state.active && !state.released &&
			!returnedResourceOwner(pass, returned, resource, contract.cleanup) &&
			!ssautil.ReturnedSameAsAny(returned, owners) {
			return state, true
		}
	}
	return state, false
}

func resourceSuccessorStates(state resourceFlowState, errorValue, resource ssa.Value) []resourceFlowState {
	successors := ssautil.FeasibleSuccessors(state.block, state.predecessor)
	result := make([]resourceFlowState, 0, len(successors))
	for _, successor := range successors {
		active := state.active
		if success, known := resourceSuccessBranch(state.block, successor, errorValue); known {
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
	if ssautil.ReturnedValueOwnsValue(returned, resource) {
		return true
	}
	for _, result := range returned.Results {
		if !ssautil.ValueDerivesFrom(result, resource, map[ssa.Value]bool{}) {
			continue
		}
		methods := types.NewMethodSet(result.Type())
		for index := range methods.Len() {
			if slices.Contains(cleanup, methods.At(index).Obj().Name()) {
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

func testProvesHTTPError(acquisition *ssa.Call, resource, errorValue ssa.Value) bool {
	// Test assertions can prove the owned-response path infeasible even though
	// the assertion package expresses that fact outside the CFG.
	// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/cmd/wfx/cmd/root/root_test.go#L154-L157
	errorAssertions, nilAssertions := httpErrorAssertions(acquisition, resource, errorValue)
	// net/http only returns a non-nil response together with an error for a
	// failed redirect policy, and its body is already closed. A fatal Error
	// assertion therefore eliminates the success path; a paired Nil assertion
	// supplies the same evidence for non-fatal assertion packages.
	for _, assertedError := range errorAssertions {
		if fatalErrorAssertion(assertedError) || errorAssertionDominatesNil(assertedError, nilAssertions) {
			return true
		}
	}
	return false
}

func httpErrorAssertions(acquisition *ssa.Call, resource, errorValue ssa.Value) ([]ssa.Instruction, []ssa.Instruction) {
	var errorAssertions, nilAssertions []ssa.Instruction
	for _, block := range acquisition.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !ssautil.InstructionMayFollow(acquisition, instruction) {
				continue
			}
			common := ssautil.InstructionCall(instruction)
			if !ssautil.HasLibraryContract(common, ssautil.ContractTestifyAssertion) {
				continue
			}
			if ssautil.CallName(common) == "Error" {
				for _, argument := range common.Args {
					if ssautil.ValueDerivesFrom(argument, errorValue, map[ssa.Value]bool{}) {
						errorAssertions = append(errorAssertions, instruction)
					}
				}
			}
			if ssautil.CallName(common) == "Nil" {
				for _, argument := range common.Args {
					if ssautil.SameValue(argument, resource) {
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
		if ssautil.InstructionDominates(assertedError, assertedNil) {
			return true
		}
	}
	return false
}

func fatalErrorAssertion(instruction ssa.Instruction) bool {
	common := ssautil.InstructionCall(instruction)
	return ssautil.HasLibraryContract(common, ssautil.ContractTestifyFatalError)
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
	comparesResourceToNil := ssautil.ValueDerivesFrom(comparison.X, resource, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.Y) ||
		ssautil.ValueDerivesFrom(comparison.Y, resource, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.X)
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
