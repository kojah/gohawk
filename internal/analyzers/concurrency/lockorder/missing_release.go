package lockorder

import (
	"fmt"
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	proofs "github.com/kojah/gohawk/internal/proof"
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
	heldAt map[*ssa.Return]lockReturnState, callers conditionalCallerSet, query lockReturnQueries,
) {
	for identity, returns := range unreleased {
		if !query.budget.Spend() {
			return
		}
		position := flow.acquiredAt[identity]
		proof := flow.proveMissingRelease(function, heldAt, callers, identity, returns, query)
		traceLockDiagnostic(flow.pass, check.LockMissingRelease, position, proof)
		if proof.state != proofs.EvidenceProven {
			continue
		}
		// Naming and related evidence are identical for every uncovered return.
		// Reuse them; a cutoff still discards the entire buffered function.
		name, related := flow.lockName(identity), flow.acquisitionEvidence(identity)
		if query.budget.Exhausted() {
			return
		}
		for _, returned := range returns {
			if !query.budget.Spend() {
				return
			}
			if returned == token.NoPos {
				returned = position
			}
			flow.releaseAttempts.trace(flow.pass, identity, returned)
			source := syntax.SourceRange(flow.pass, returned)
			check.Report(flow.pass, check.LockMissingRelease, analysis.Diagnostic{
				Pos: source.Pos(), End: source.End(),
				Message: fmt.Sprintf("lock %s is not released on this return path", name),
				Related: related,
			})
		}
	}
}

// proveMissingRelease consumes held-return witnesses only after the bounded
// function walk completes. A possible acquisition or unwitnessed local release
// leaves ownership uncertain; caller-transfer contracts retain their order.
func (flow lockFlowContext) proveMissingRelease(
	function *ssa.Function, heldAt map[*ssa.Return]lockReturnState, callers conditionalCallerSet,
	identity string, returns []token.Pos, query lockReturnQueries,
) lockDiagnosticProof {
	unknown := lockDiagnosticProof{proofs.EvidenceUnknown, lockReasonLockStateBudgetExhausted}
	if !query.budget.Spend() {
		return unknown
	}
	if flow.uncertainGuards[identity] {
		return lockDiagnosticProof{proofs.EvidenceUnknown, lockReasonLoadedAcquisitionGuardUnknown}
	}
	values := flow.lockValues[identity]
	private := slices.ContainsFunc(values, func(value ssa.Value) bool {
		return query.budget.Spend() && privateMutexOnly(value, query.budget)
	})
	if query.budget.Exhausted() {
		return unknown
	}
	if private {
		return lockDiagnosticProof{proofs.EvidenceDisproven, lockReasonPrivateMutexOnly}
	}
	if !flow.released[identity] {
		return lockDiagnosticProof{proofs.EvidenceUnknown, lockReasonReleaseOwnershipUnknown}
	}
	held := query.acquiresForCaller(function, flow.acquisitions[identity], heldAt, identity)
	if query.budget.Exhausted() {
		return unknown
	}
	if held.proven {
		return lockDiagnosticProof{proofs.EvidenceDisproven, held.reason}
	}
	caller := query.conditionalCallerRelease(function, values, heldAt, identity, callers)
	if query.budget.Exhausted() {
		return unknown
	}
	if caller.proven {
		return lockDiagnosticProof{proofs.EvidenceDisproven, caller.reason}
	}
	if len(returns) == 0 {
		return lockDiagnosticProof{proofs.EvidenceDisproven, lockReasonNone}
	}
	return lockDiagnosticProof{proofs.EvidenceProven, lockReasonUnreleasedReturn}
}
