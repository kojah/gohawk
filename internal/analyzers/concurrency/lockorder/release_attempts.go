package lockorder

import (
	"go/token"
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A missing release is found at a return, but the calls that might have
// released the lock were asked about earlier in the walk, before any return
// was known to be reported. While tracing, those unproven release questions
// are kept per lock and replayed as evidence on each reported return, so a
// trace of the reported line names the helpers that were asked and the
// completion search's reason for each. Nothing is kept when tracing is off.

type releaseAttempt struct {
	position    token.Pos
	instruction string
	// source names the evidence consulted: the completion search, or the
	// helper's complete summarized lock sequence.
	source string
	reason ssaflow.EvidenceReason
	// possible records that the call releases the lock on some path.
	possible bool
}

type releaseAttempts struct {
	byLock map[string][]releaseAttempt
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
	identity string, instruction ssa.Instruction, lock ssa.Value, proof ssaflow.CompletionProof, possible bool,
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
			reason: ssaflow.EvidenceNotFound, possible: true,
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
