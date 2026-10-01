package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// A resource lifetime policy result carries the analyzer's final disposition
// together with the stable reason exposed by decision tracing. SSA and fact
// queries establish evidence; this type owns only the reporting policy that
// combines those proofs.
type resourceLifetimePolicyResult struct {
	// state proves whether this candidate permits a diagnostic, not whether
	// cleanup occurred. Policy exclusions are disproven; opaque ownership is
	// unknown. Both suppress reporting without claiming the same guarantee.
	state  ssaflow.EvidenceState
	reason resourceLifetimeReason
	// leak is the normal return the flow reached with the resource still
	// owed: the witness a reported diagnostic cites.
	leak *ssa.Return
}

func acceptedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{state: ssaflow.EvidenceDisproven, reason: reason}
}

func unknownResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{state: ssaflow.EvidenceUnknown, reason: reason}
}

func reportedResourceLifetime(reason resourceLifetimeReason) resourceLifetimePolicyResult {
	return resourceLifetimePolicyResult{state: ssaflow.EvidenceProven, reason: reason}
}
