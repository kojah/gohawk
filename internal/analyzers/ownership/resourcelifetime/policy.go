package resourcelifetime

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
	resourceReasonCanceledAcquisition resourceLifetimeReason = "context-canceled-before-acquisition"
	resourceReasonReleaseProven       resourceLifetimeReason = "release-proven"
	resourceReasonUnownedReturn       resourceLifetimeReason = "unowned-return"
	resourceReasonOpaqueConsumption   resourceLifetimeReason = "opaque-consumption"
)

func acceptedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{reason: reason}
}

func reportedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{reason: reason, report: true}
}
