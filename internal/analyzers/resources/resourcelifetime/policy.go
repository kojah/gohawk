package resourcelifetime

import "golang.org/x/tools/go/ssa"

// A resource lifetime policy result carries the analyzer's final disposition
// together with the stable reason exposed by decision tracing. SSA and fact
// queries establish evidence; this type owns only the reporting policy that
// combines those proofs.
type resourceLifetimePolicyResult struct {
	reason resourceLifetimeReason
	report bool
	// leak is the normal return the flow reached with the resource still
	// owed: the witness a reported diagnostic cites.
	leak *ssa.Return
}

func acceptedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{reason: reason}
}

func reportedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{reason: reason, report: true}
}
