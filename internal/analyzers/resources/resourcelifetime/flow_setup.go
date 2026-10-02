package resourcelifetime

import "github.com/kojah/gohawk/internal/ssaflow"

// Flow setup collects bounded owner, collection and deferred-release evidence
// before the path proof starts. A possible prior release stops at uncertainty;
// partial discovery cannot supply classifiers with an authoritative census.
func (analysis *resourceAnalysis) prepareResourceFlow() resourceProof {
	// This query uses anywhere coverage, not every-return settlement. A
	// dominating defer may release a later acquisition through captured storage.
	deferred := analysis.proveDeferredBeforeAcquisitionWithin(analysis.acquisition, analysis.budget(releaseSearchBudget))
	if deferred.State == ssaflow.EvidenceUnknown {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: deferred.Reason}
	}
	if deferred.Proven() {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonPriorDeferMayRelease}
	}
	owners := analysis.discoverResourceOwnersWithin(analysis.budget(releaseSearchBudget))
	if !owners.Proven() {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	analysis.collection = analysis.localCollection()
	guarded := analysis.discoverResultGuardedDefersWithin(analysis.budget(releaseSearchBudget))
	if guarded.State == ssaflow.EvidenceUnknown {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: guarded.Reason}
	}
	prior := analysis.provePriorCleanupWithin(analysis.acquisition, analysis.budget(releaseSearchBudget))
	if prior.State == ssaflow.EvidenceUnknown {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: prior.Reason}
	}
	if prior.Proven() {
		analysis.emitAction(prior.Instruction, actionUnknown, prior.Reason)
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonOpaqueConsumption}
	}
	return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonNone}
}
