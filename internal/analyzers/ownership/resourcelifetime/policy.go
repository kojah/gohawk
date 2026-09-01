package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A resource lifetime policy result carries the analyzer's final disposition
// together with the stable reason exposed by decision tracing. SSA and fact
// queries establish evidence; this type owns only the reporting policy that
// combines those proofs.
type resourceLifetimePolicyResult struct {
	reason resourceLifetimeReason
	report bool
}

type resourceLifetimeReason string

const (
	resourceReasonReleaseProven          resourceLifetimeReason = "release-proven"
	resourceReasonCompleteTimerLifecycle resourceLifetimeReason = "complete-timer-lifecycle"
	resourceReasonUnownedReturn          resourceLifetimeReason = "unowned-return"
)

func evaluateResourceLifetime(
	pass *analysis.Pass,
	evidence *lifecyclefacts.LifecycleEvidence,
	call *ssa.Call,
	resource ssa.Value,
	contract resourceContract,
	completeTimer bool,
) resourceLifetimePolicyResult {
	// Preserve flow evaluation even when source evidence proves the complete
	// timer idiom. Besides keeping evidence tracing stable, this prevents the
	// policy layer from changing which underlying proofs are queried.
	flow := evaluateResourceFlow(pass, evidence, call, resource, contract)
	if completeTimer {
		return acceptedResourceLifetime(resourceReasonCompleteTimerLifecycle)
	}
	return flow
}

func acceptedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{reason: reason}
}

func reportedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{reason: reason, report: true}
}
