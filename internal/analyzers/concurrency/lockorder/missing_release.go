package lockorder

import (
	"fmt"
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Missing-release policy consumes held-return witnesses after the bounded
// function walk completes. Caller ownership and uncertain acquisition guards
// may suppress a witness; this file does not run another held-state flow.

// A lock is reported only when some path releases it and another returns with
// it held. Without a witnessed release, ownership may belong to a caller.
// Explicit successful-return and conditional caller-release contracts likewise
// distinguish a transferred critical section from an abandoned acquisition.
func (flow lockFlowContext) reportMissingReleases(
	function *ssa.Function, unreleased map[string][]token.Pos,
	heldAt map[*ssa.Return]lockReturnState, callers conditionalCallerSet,
) {
	for identity, returns := range unreleased {
		position := flow.acquiredAt[identity]
		proof := flow.proveMissingRelease(function, heldAt, callers, identity, returns)
		traceLockDiagnostic(flow.pass, check.LockMissingRelease, position, proof)
		if proof.state != ssaflow.EvidenceProven {
			continue
		}
		for _, returned := range returns {
			if returned == token.NoPos {
				returned = position
			}
			flow.releaseAttempts.trace(flow.pass, identity, returned)
			source := syntax.SourceRange(flow.pass, returned)
			check.Report(flow.pass, check.LockMissingRelease, analysis.Diagnostic{
				Pos: source.Pos(), End: source.End(),
				Message: fmt.Sprintf("lock %s is not released on this return path", flow.lockName(identity)),
				Related: flow.acquisitionEvidence(identity),
			})
		}
	}
}

// proveMissingRelease consumes held-return witnesses only after the bounded
// function walk completes. A possible acquisition or unwitnessed local release
// leaves ownership uncertain; caller-transfer contracts retain their order.
func (flow lockFlowContext) proveMissingRelease(
	function *ssa.Function, heldAt map[*ssa.Return]lockReturnState, callers conditionalCallerSet, identity string, returns []token.Pos,
) lockDiagnosticProof {
	if flow.uncertainGuards[identity] {
		return lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonLoadedAcquisitionGuardUnknown}
	}
	values := flow.lockValues[identity]
	if slices.ContainsFunc(values, privateMutexOnly) {
		return lockDiagnosticProof{ssaflow.EvidenceDisproven, lockReasonPrivateMutexOnly}
	}
	if !flow.released[identity] {
		return lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonReleaseOwnershipUnknown}
	}
	if proof := acquiresForCaller(function, flow.acquisitions[identity], heldAt, identity); proof.proven {
		return lockDiagnosticProof{ssaflow.EvidenceDisproven, proof.reason}
	}
	if proof := conditionalCallerRelease(function, values, heldAt, identity, callers); proof.proven {
		return lockDiagnosticProof{ssaflow.EvidenceDisproven, proof.reason}
	}
	if len(returns) == 0 {
		return lockDiagnosticProof{ssaflow.EvidenceDisproven, lockReasonNone}
	}
	return lockDiagnosticProof{ssaflow.EvidenceProven, lockReasonUnreleasedReturn}
}
