package lockorder

import (
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// Lock release searches share one query path and the function traversal pool.
// Their coverage remains caller policy; interruption invalidates the function
// instead of manufacturing a release or retaining partial ordering evidence.

// lockCompletionBudget bounds one "does this callee release my lock?" question
// by the instructions it may examine. Mutually recursive helpers make the
// number of routes through a call graph explode, and an answer the cycle guard
// cuts short cannot be memoized, so an unbounded search re-walks the graph once
// per route. A package of eighteen mutually recursive methods with four calls
// each took over seven seconds before this bound and is instant with it.
const lockCompletionBudget = 250_000

// releaseSettled projects an exact completion proof into the launch form this
// release policy accepts. Interrupted searches remain unavailable for the whole
// function; they must never stand in for a proven release.
func releaseSettled(proof ssaflow.CompletionProof, reason ssaflow.EvidenceReason) bool {
	return proof.Proven() && proof.Reason == reason
}

type lockReleaseQueries struct {
	evidence *lifecycle.LocalEvidence
	budget   *ssaflow.SearchBudget
	limit    int
	cutoff   ssaflow.Proof
}

func newLockReleaseQueries(evidence *lifecycle.LocalEvidence, budget *ssaflow.SearchBudget) *lockReleaseQueries {
	return &lockReleaseQueries{evidence: evidence, budget: budget, limit: lockCompletionBudget}
}

func (queries *lockReleaseQueries) query(
	instruction ssa.Instruction, target ssa.Value, coverage lifecycle.CompletionCoverage, observer ssaflow.Observer,
) ssaflow.CompletionProof {
	if queries.cutoff.Reason == ssaflow.EvidenceBudgetExhausted || !queries.budget.Spend() {
		return queries.interrupted()
	}
	child := queries.budget.Within(queries.limit)
	if observer != nil {
		child.Observed(observer)
	}
	proof := queries.evidence.Completion(lifecycle.CompletionRequest{
		Instruction: instruction, Target: target, Methods: []string{"Unlock", "RUnlock"}, Coverage: coverage, Budget: child,
	})
	if child.Exhausted() || child.PoolExhausted() || proof.Reason == ssaflow.EvidenceBudgetExhausted {
		return queries.interrupted()
	}
	return proof
}

func (queries *lockReleaseQueries) interrupted() ssaflow.CompletionProof {
	queries.cutoff = ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
	return ssaflow.CompletionProof{Proof: queries.cutoff}
}

func (queries *lockReleaseQueries) identities(held []string) []string {
	for range held {
		if !queries.budget.Spend() {
			queries.interrupted()
			return nil
		}
	}
	return slices.Clone(held)
}

func (flow lockFlowContext) possiblyDeferredUnlock(acquisition ssa.Instruction, values []ssa.Value) bool {
	for _, deferred := range flow.defers {
		if !flow.releases.budget.Spend() {
			return false
		}
		dominates := ssaflow.InstructionDominatesWithin(deferred, acquisition, flow.releases.budget)
		if flow.releases.budget.Exhausted() {
			return false
		}
		if !dominates {
			continue
		}
		for _, value := range values {
			if !flow.releases.budget.Spend() {
				return false
			}
			proof := flow.releases.query(deferred, value, lifecycle.CoverageAnywhere, nil)
			if flow.releases.cutoff.Reason == ssaflow.EvidenceBudgetExhausted {
				return false
			}
			if releaseSettled(proof, ssaflow.EvidenceDeferredCompletion) {
				// A defer registered before acquisition can conditionally release the
				// exact lock using state established after Lock. Without proving the
				// deferred guard false, a missing-release defect is uncertain. Telekom's
				// artifact store uses this rollback shape:
				// https://github.com/telekom/k8s-breakglass/blob/9b078a5e78c5663cfdf8b7711ff24fc2a6aaee59/pkg/artifacts/storage/local/local.go#L265-L329
				return true
			}
		}
	}
	return false
}

// Called and spawned completion share one exact held-lock transition. The
// launch form selects the proof reason; only synchronous calls retain a
// possible-release witness when exact completion fails. Defers register future
// cleanup through recordDeferredUnlocks rather than consuming held locks here.
func (flow lockFlowContext) transferCompletedUnlocks(instruction ssa.Instruction, state lockFlowState) []string {
	var reason ssaflow.EvidenceReason
	switch instruction.(type) {
	case *ssa.Call:
		reason = ssaflow.EvidenceCalledCompletion
	case *ssa.Go:
		reason = ssaflow.EvidenceStartedCompletion
	default:
		return state.held
	}
	held, guards := state.held, state.guards
	for _, identity := range flow.releases.identities(held) {
		if !flow.releases.budget.Spend() {
			return held
		}
		for _, value := range flow.lockValues[identity] {
			proof := flow.releases.query(instruction, value, lifecycle.CoverageEveryReturn, nil)
			if flow.releases.cutoff.Reason == ssaflow.EvidenceBudgetExhausted {
				return held
			}
			if !releaseSettled(proof, reason) {
				if reason == ssaflow.EvidenceCalledCompletion {
					flow.recordPossibleCalledRelease(instruction, identity, value, proof)
					if flow.releases.cutoff.Reason == ssaflow.EvidenceBudgetExhausted {
						return held
					}
				}
				continue
			}
			// Exact helper completion consumes the held obligation. Spawned completion
			// transfers it to the worker only when every normal worker return releases.
			// https://github.com/grpc/grpc-go/blob/9f8027448a64b6446d0c7256a1efe907b1cb6b1b/clientconn.go#L416-L419
			// https://github.com/nats-io/nats.go/blob/850f889cf3d63bfd1a549ab9af59f0145146fb41/js.go#L906-L976
			// https://github.com/grpc/grpc-go/blob/9f8027448a64b6446d0c7256a1efe907b1cb6b1b/clientconn.go#L1071
			flow.released[identity] = true
			held = releaseLock(held, identity)
			delete(guards, identity)
			break
		}
	}
	return held
}

func (flow lockFlowContext) recordPossibleCalledRelease(
	instruction ssa.Instruction, identity string, value ssa.Value, proof ssaflow.CompletionProof,
) {
	possible := flow.mayRelease(instruction, value)
	if flow.releases.cutoff.Reason == ssaflow.EvidenceBudgetExhausted {
		return
	}
	flow.releaseAttempts.record(identity, instruction, value, proof, possible)
	// A callee that releases the lock on some paths and not others
	// leaves it held but no longer proven held. A return that still
	// holds it stays reportable, deliberately, so a conditional
	// handoff cannot hide a leak. A later acquisition of it must
	// not be called recursive, because that claim needs positive
	// evidence the lock IS held. vekil wraps its mutex in a type
	// whose Unlock returns early on a nil receiver, which left a
	// Lock and Unlock paired inside a loop body reported as a
	// recursive acquisition on the next iteration:
	// https://github.com/sozercan/vekil/blob/842f12f7875143274378fcbb80d411295edf3d28/proxy/route_executor.go#L697
	if possible {
		flow.unprovenRelease[identity] = true
	}
}

func (flow lockFlowContext) recordDeferredUnlocks(
	instruction ssa.Instruction,
	held, deferred []string,
) []string {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return deferred
	}
	for _, identity := range flow.releases.identities(held) {
		if !flow.releases.budget.Spend() {
			return deferred
		}
		probe := analysisTrace.For(flow.pass, "lockorder", string(check.LockMissingRelease), flow.acquiredAt[identity])
		for _, value := range flow.lockValues[identity] {
			// A deferred literal that releases on some path makes the release
			// data-dependent, typically through an "already unlocked" flag.
			// Missing-release diagnostics need the release to be impossible, so
			// this asks only whether the defer may unlock.
			proof := flow.releases.query(instruction, value, lifecycle.CoverageAnywhere, probe.Observer())
			if flow.releases.cutoff.Reason == ssaflow.EvidenceBudgetExhausted {
				return deferred
			}
			if releaseSettled(proof, ssaflow.EvidenceDeferredCompletion) {
				if !slices.Contains(deferred, identity) {
					probe.Evidence(analysisTrace.Step{
						Reason: lockReasonDeferredReleaseProven.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: instruction.Pos(),
					})
				}
				flow.released[identity] = true
				deferred = appendUniqueString(deferred, identity)
				break
			}
		}
	}
	return deferred
}

// mayRelease reports whether the call releases the lock on at least one path.
// It is the weaker companion to the proof transferCompletedUnlocks requires, and
// answers only whether the caller may still claim the lock is held.
func (flow lockFlowContext) mayRelease(instruction ssa.Instruction, value ssa.Value) bool {
	proof := flow.releases.query(instruction, value, lifecycle.CoverageAnywhere, nil)
	return proof.Proven() && proof.Reason == ssaflow.EvidenceCalledCompletion
}
