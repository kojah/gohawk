// Package resourcelifetime implements the resourcelifetime gohawk analyzer.
package resourcelifetime

import (
	"errors"
	"fmt"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var resourceSummaries = summaries.Select(summaries.Requirements{Results: true, Lifecycle: true})

func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "resourcelifetime",
		Doc:      "checks owned files, SQL handles, HTTP responses, and compressors are released on every path",
		Requires: resourceSummaries.Requires(),
		Run:      runResourceLifetime,
	}
}

type resourceLifetimeSettings struct {
	catalog []resourceContract
}

func runResourceLifetime(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	settings := resourceLifetimeSettings{catalog: resourceContracts()}
	provider := resourceSummaries.Provider(pass)
	// Acquisition contracts identify both the owned result and its required
	// cleanup action. Reporting is deferred until path analysis proves that the
	// action or a recognized ownership transfer is absent on a normal return.
	for _, function := range functions {
		evidence, available := provider.LifecycleEvidence("resourcelifetime", string(check.ResourceRelease))
		if available != summaries.Available {
			return nil, errors.New("resourcelifetime: lifecycle summary prerequisite unavailable")
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				contract, ok := resourceContractFor(call.Common(), settings)
				if !ok {
					contract, ok = ownedResultContract(evidence, call, settings)
				}
				if !ok {
					continue
				}
				resource := ssaflow.CallResult(call, contract.result)
				if resource == nil {
					continue
				}
				// Exemption from leak cleanup does not make a closed in-memory
				// writer usable again. Invalidation has its own API contract.
				reportUsesAfterRelease(pass, resourceSummaries.Provider(pass), function, call, resource, contract)
				if memoryWriterExempt(call, contract) {
					continue
				}
				evidence.ForCandidate(call.Pos())
				result := evaluateResourceFlow(pass, evidence, call, resource, contract)
				emitResourceDecision(pass, function, call, resource, contract, result)
				if result.report {
					message := "owned resource from %s.%s is not released on every return path"
					if contract.retained {
						message = "resource held by the result of %s.%s is dropped on some return path"
					}
					source := syntax.SourceRange(pass, call.Pos())
					check.Report(pass, check.ResourceRelease, analysis.Diagnostic{
						Pos:     source.Pos(),
						End:     source.End(),
						Message: fmt.Sprintf(message, syntax.ShortPackageName(contract.packagePath), contract.name),
						Related: missingReleaseEvidence(pass, call, contract.result, result.leak),
					})
				}
			}
		}
	}
	return nil, nil
}

func emitResourceDecision(
	pass *analysis.Pass,
	function *ssa.Function,
	call *ssa.Call,
	resource ssa.Value,
	contract resourceContract,
	result resourceLifetimePolicyResult,
) {
	probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos())
	if !probe.Enabled() {
		return
	}
	outcome := analysisTrace.OutcomeAccepted
	if result.report {
		outcome = analysisTrace.OutcomeRejected
	} else if result.reason == resourceReasonHeadAcquisition || result.reason == resourceReasonHeaderOnlyAcquisition {
		outcome = analysisTrace.OutcomeUnknown
	}
	details := map[string]string{"acquisition": contract.packagePath + "." + contract.name}
	if resource != nil && resource.Type() != nil {
		details["resource_type"] = resource.Type().String()
	}
	probe.Decision(analysisTrace.Step{
		Reason:   result.reason.String(),
		Outcome:  outcome,
		Pos:      call.Pos(),
		Function: function.String(),
		Details:  details,
	})
}
