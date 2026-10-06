// Package resourcelifetime implements the resourcelifetime gohawk analyzer.
package resourcelifetime

import (
	"errors"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
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
				contracts := resourceContractsFor(call.Common(), settings)
				if len(contracts) == 0 {
					if contract, ok := ownedResultContract(evidence, call, settings); ok {
						contracts = append(contracts, contract)
					}
				}
				for _, contract := range contracts {
					checkAcquisition(pass, evidence, function, call, contract)
				}
			}
		}
	}
	return nil, nil
}

// checkAcquisition proves or reports one owned result of an acquisition.
func checkAcquisition(pass *analysis.Pass, evidence *lifecyclefacts.LifecycleEvidence, function *ssa.Function, call *ssa.Call, contract resourceContract) {
	resource := ssacall.CallResult(call, contract.result)
	if resource == nil {
		return
	}
	result := evaluateResourceFlow(pass, evidence, call, resource, contract)
	emitResourceDecision(pass, function, call, resource, contract, result)
	if result.state != proofs.EvidenceProven {
		return
	}
	acquisition := syntax.ShortPackageName(contract.packagePath) + "." + contract.name
	if contract.role != "" {
		acquisition += " (" + contract.role + ")"
	}
	message := "owned resource from " + acquisition + " is not released on every return path"
	if contract.retained {
		message = "resource held by the result of " + acquisition + " is dropped on some return path"
	}
	source := syntax.SourceRange(pass, call.Pos())
	check.Report(pass, check.ResourceRelease, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(), Message: message,
		Related: missingReleaseEvidence(pass, call, contract.result, result.leak),
	})
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
	outcome := analysisTrace.DiagnosticOutcome(result.state)
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

// missingReleaseEvidence cites the return the flow walk reached with the
// resource still owed, naming the variable the acquisition assigned. The
// witness is the proof's own, so the report and its evidence cannot disagree.
func missingReleaseEvidence(pass *analysis.Pass, call *ssa.Call, result int, leak *ssa.Return) []analysis.RelatedInformation {
	subject := "the resource"
	if name := syntax.AssignedName(pass, call.Pos(), result); name != "" {
		subject = "`" + name + "`"
	}
	return check.ReturnEvidence(pass, call, leak, "releasing "+subject)
}

// A resource lifetime policy result carries the analyzer's final disposition
// together with the stable reason exposed by decision tracing. SSA and fact
// queries establish evidence; this type owns only the reporting policy that
// combines those proofs.
type resourceLifetimePolicyResult struct {
	// state proves whether this candidate permits a diagnostic, not whether
	// cleanup occurred. Policy exclusions are disproven; opaque ownership is
	// unknown. Both suppress reporting without claiming the same guarantee.
	state  proofs.EvidenceState
	reason resourceLifetimeReason
	// leak is the normal return the flow reached with the resource still
	// owed: the witness a reported diagnostic cites.
	leak *ssa.Return
}

func acceptedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{state: proofs.EvidenceDisproven, reason: reason}
}

func unknownResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{state: proofs.EvidenceUnknown, reason: reason}
}

func reportedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{state: proofs.EvidenceProven, reason: reason}
}
