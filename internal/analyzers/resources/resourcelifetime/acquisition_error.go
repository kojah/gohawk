package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Acquisition-error assertions supply the existing test-contract exclusion.
// Census, ordering and argument provenance share one allowance; partial
// evidence cannot establish that the successful resource path is unavailable.

func proveAcquisitionErrorWithin(acquisition *ssa.Call, resource, errorValue ssa.Value, httpResponse bool, budget *ssaflow.SearchBudget) resourceProof {
	// Test assertions can prove the owned-resource path infeasible even though
	// the assertion package expresses that fact outside the CFG.
	// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/cmd/wfx/cmd/root/root_test.go#L154-L157
	errorAssertions, nilAssertions := acquisitionErrorAssertionsWithin(acquisition, resource, errorValue, budget)
	// A fatal Error assertion stops the test unless the acquisition failed,
	// which is the same evidence as an `if err != nil { return }` guard for any
	// acquisition. The non-fatal form is accepted only for net/http, whose
	// paired Nil assertion carries the extra fact that a response returned
	// together with an error has an already-closed body.
	if resourceFlowExhausted(budget) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	for _, assertedError := range errorAssertions {
		if !budget.Spend() {
			break
		}
		if fatalErrorAssertion(assertedError) || httpResponse && errorAssertionDominatesNilWithin(assertedError, nilAssertions, budget) {
			return carriedValueProof(true, resourceReasonReleaseProven, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

func acquisitionErrorAssertionsWithin(
	acquisition *ssa.Call,
	resource, errorValue ssa.Value,
	budget *ssaflow.SearchBudget,
) ([]ssa.Instruction, []ssa.Instruction) {
	var errorAssertions, nilAssertions []ssa.Instruction
	for instruction := range ssaflow.InstructionsWithin(acquisition.Parent(), budget) {
		common := ssaflow.InstructionCall(instruction)
		errorClaim := ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyErrorClaim)
		nilClaim := ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyNilClaim)
		if !errorClaim && !nilClaim {
			continue
		}
		if !ssaflow.InstructionMayFollowWithin(acquisition, instruction, budget) {
			continue
		}
		if errorClaim {
			for _, argument := range common.Args {
				if !budget.Spend() {
					return nil, nil
				}
				if heapmodel.ValueDerivesFromWithin(argument, errorValue, budget) {
					errorAssertions = append(errorAssertions, instruction)
				}
			}
		}
		if nilClaim {
			for _, argument := range common.Args {
				if !budget.Spend() {
					return nil, nil
				}
				if budget.Spend() && heapmodel.MayAlias(argument, resource) {
					nilAssertions = append(nilAssertions, instruction)
				}
			}
		}
	}
	// An interrupted census publishes neither assertion list. Alias dispatch is
	// charged here; graph construction and alias-query internals are independent.
	if resourceFlowExhausted(budget) {
		return nil, nil
	}
	return errorAssertions, nilAssertions
}

func errorAssertionDominatesNilWithin(assertedError ssa.Instruction, nilAssertions []ssa.Instruction, budget *ssaflow.SearchBudget) bool {
	for _, assertedNil := range nilAssertions {
		if ssaflow.InstructionDominatesWithin(assertedError, assertedNil, budget) {
			return true
		}
	}
	return false
}

func fatalErrorAssertion(instruction ssa.Instruction) bool {
	common := ssaflow.InstructionCall(instruction)
	return ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyFatalError)
}
