package lockorder

import (
	"fmt"
	"go/token"
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"github.com/kojah/gohawk/internal/reporting/check"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
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

// A missing release is found at a return, but the calls that might have
// released the lock were asked about earlier in the walk, before any return
// was known to be reported. While tracing, those unproven release questions
// are kept per lock and replayed as evidence on each reported return, so a
// trace of the reported line names the helpers that were asked and the
// completion search's reason for each. Observed mutex actions are deduplicated
// across visited states; their merged observations are not one execution path.
// Uncertain alias releases are traced once when consumed. Nothing is kept when
// tracing is off.

type releaseAttempt struct {
	position    token.Pos
	instruction string
	// source names the evidence consulted: the completion search, or the
	// helper's complete summarized lock sequence.
	source string
	reason proofs.EvidenceReason
	// possible records that the call releases the lock on some path.
	possible bool
}

type releaseAttempts struct {
	byLock          map[string][]releaseAttempt
	actions         []mutexActionTrace
	unknownReleases []uncertainReleaseTrace
}

type uncertainReleaseTrace struct {
	identity           string
	acquired, released token.Pos
}

func (attempts *releaseAttempts) traceUnknownRelease(pass *analysis.Pass, identity string, acquired token.Pos, instruction ssa.Instruction) {
	if attempts == nil {
		return
	}
	release := uncertainReleaseTrace{identity: identity, acquired: acquired, released: instruction.Pos()}
	if slices.Contains(attempts.unknownReleases, release) {
		return
	}
	attempts.unknownReleases = append(attempts.unknownReleases, release)
	analysisTrace.For(pass, "lockorder", string(check.LockMissingRelease), acquired).Evidence(analysisTrace.Step{
		Reason: lockReasonReleaseIdentityUnknown.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: instruction.Pos(),
	})
}

// Recorded actions explain which exact identity the flow acquired or released.
// A deferred action registers future work rather than executing it now.
type mutexActionTrace struct {
	position              token.Pos
	instruction, identity string
	operation             mutexOperation
	deferred              bool
}

func (attempts *releaseAttempts) recordAction(instruction ssa.Instruction, effect mutexEffect) {
	if attempts == nil {
		return
	}
	_, deferred := instruction.(*ssa.Defer)
	action := mutexActionTrace{
		position: instruction.Pos(), instruction: instruction.String(), identity: effect.identity,
		operation: effect.operation, deferred: deferred,
	}
	if !slices.Contains(attempts.actions, action) {
		attempts.actions = append(attempts.actions, action)
	}
}

// newReleaseAttempts returns a recorder while missing-release tracing is on,
// and nil otherwise, which records nothing.
func newReleaseAttempts() *releaseAttempts {
	if !analysisTrace.Enabled("lockorder", string(check.LockMissingRelease)) {
		return nil
	}
	return &releaseAttempts{byLock: map[string][]releaseAttempt{}}
}

// record keeps one unproven release question about a call that was handed
// the lock or the object that owns it, since only such a call could have
// released it. Direct mutex operations are decided elsewhere. The walk
// revisits instructions in several states, so a repeated question is kept
// once.
func (attempts *releaseAttempts) record(
	identity string, instruction ssa.Instruction, lock ssa.Value, proof proofs.CompletionProof, possible bool,
) {
	if attempts == nil || !receivesLock(instruction, lock) {
		return
	}
	attempts.add(identity, releaseAttempt{
		position: instruction.Pos(), instruction: instruction.String(), source: "completion", reason: proof.Reason, possible: possible,
	})
}

// receivesLock reports whether a call that is not itself a mutex operation
// is handed the lock or its owner.
func receivesLock(instruction ssa.Instruction, lock ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if _, _, _, direct := mutexAction(instruction); direct {
		return false
	}
	candidates := []ssa.Value{lock}
	if owner, ok := lockOwner(lock); ok {
		candidates = append(candidates, owner)
	}
	operands := append([]ssa.Value{ssaflow.CallReceiver(common)}, common.Args...)
	for _, operand := range operands {
		if operand != nil && heapmodel.MayAliasAny(operand, candidates) {
			return true
		}
	}
	return false
}

// recordSummarized keeps a helper whose summarized lock sequence releases a
// held lock on some path but leaves it held after the call.
func (attempts *releaseAttempts) recordSummarized(instruction ssa.Instruction, effects []mutexEffect, before, after []string) {
	if attempts == nil {
		return
	}
	for _, effect := range effects {
		if effect.operation != mutexRelease || !slices.Contains(before, effect.identity) || !slices.Contains(after, effect.identity) {
			continue
		}
		attempts.add(effect.identity, releaseAttempt{
			position: instruction.Pos(), instruction: instruction.String(), source: "summary",
			reason: proofs.EvidenceNotFound, possible: true,
		})
	}
}

func (attempts *releaseAttempts) add(identity string, attempt releaseAttempt) {
	if slices.Contains(attempts.byLock[identity], attempt) {
		return
	}
	attempts.byLock[identity] = append(attempts.byLock[identity], attempt)
}

// trace emits the recorded questions for one lock against a reported return.
func (attempts *releaseAttempts) trace(pass *analysis.Pass, identity string, returned token.Pos) {
	if attempts == nil {
		return
	}
	probe := analysisTrace.For(pass, "lockorder", string(check.LockMissingRelease), returned)
	for _, action := range attempts.actions {
		operation := "acquire"
		if action.operation == mutexRelease {
			operation = "release"
		}
		probe.Evidence(analysisTrace.Step{
			Reason: lockReasonMutexActionObserved.String(), Outcome: analysisTrace.OutcomeObserved, Pos: action.position,
			Details: map[string]string{
				"instruction": action.instruction, "lock": action.identity, "operation": operation,
				"deferred": strconv.FormatBool(action.deferred),
			},
		})
	}
	for _, attempt := range attempts.byLock[identity] {
		probe.Evidence(analysisTrace.Step{
			Reason: lockReasonHelperReleaseUnproven.String(), Outcome: analysisTrace.OutcomeRejected, Pos: attempt.position,
			Details: map[string]string{
				"instruction": attempt.instruction, "source": attempt.source, "completion": attempt.reason.String(), "lock": identity,
				"releases_on_some_path": strconv.FormatBool(attempt.possible),
			},
		})
	}
}
