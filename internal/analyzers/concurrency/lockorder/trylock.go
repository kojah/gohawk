package lockorder

// Discarded Try acquisitions.
//
// TryLock and TryRLock report whether they took the lock, and that answer is
// the only evidence the caller has. Discarding it leaves the code unable to
// tell a held lock from a contended one, so every guarded access that follows
// is unsynchronized and a matching Unlock is a fatal unlock of an unlocked
// mutex.
//
// The proof stays mechanical on purpose: a Try acquisition whose receiver is
// proven to be a concrete sync.Mutex or sync.RWMutex, and whose SSA result has
// no consumers. Demanding the sync receiver is what keeps a project's own
// TryLock method out of the check, since such a method carries no unlock
// obligation and discarding its answer is ordinary use.

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// tryLockProof is the outcome of judging one instruction. It carries the
// reason so tracing and reporting read the same decision rather than
// recomputing it.
type tryLockProof struct {
	reason   string
	method   string
	identity string
	discard  bool
}

func reportDiscardedTryLocks(pass *analysis.Pass, function *ssa.Function) {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			proof, relevant := discardedTryLockProof(call)
			if !relevant {
				continue
			}
			traceTryLockDecision(pass, call, proof)
			if !proof.discard {
				continue
			}
			check.Reportf(pass, check.LockDiscardedTryLock, call.Pos(),
				"%s result on %s is discarded, so the lock may not be held", proof.method, proof.identity)
		}
	}
}

// discardedTryLockProof judges one call. The second result reports whether the
// call is a Try acquisition worth explaining at all, so callers can skip the
// overwhelming majority of instructions without building a proof for them.
func discardedTryLockProof(call *ssa.Call) (tryLockProof, bool) {
	common := call.Common()
	if common == nil {
		return tryLockProof{}, false
	}
	method := ssaflow.CallName(common)
	if method != "TryLock" && method != "TryRLock" {
		return tryLockProof{}, false
	}
	proof := tryLockProof{method: method}
	receiver := concreteMutexReceiver(ssaflow.CallReceiver(common))
	if receiver == nil {
		// A TryLock on anything but a proven sync mutex is some other type's
		// method. It promises nothing about a lock, so its result is free to
		// be ignored.
		proof.reason = "receiver-not-a-sync-mutex"
		return proof, true
	}
	proof.identity = lockIdentityOf(receiver)
	if proof.identity == "" {
		proof.reason = "unresolved-lock-identity"
		return proof, true
	}
	if tryResultConsumed(call) {
		proof.reason = "result-consumed"
		return proof, true
	}
	proof.reason = "result-discarded"
	proof.discard = true
	return proof, true
}

// tryResultConsumed reports whether anything reads the acquisition's answer.
// Any consumer counts, including a return or an argument: once the value
// leaves this function the decision belongs to whoever receives it.
func tryResultConsumed(call *ssa.Call) bool {
	referrers := call.Referrers()
	return referrers != nil && len(*referrers) > 0
}

func traceTryLockDecision(pass *analysis.Pass, call *ssa.Call, proof tryLockProof) {
	checkID := string(check.LockDiscardedTryLock)
	probe := trace.For(pass, "lockorder", checkID, call.Pos())
	if !probe.Enabled() {
		return
	}
	step := trace.Step{
		Reason:   proof.reason,
		Outcome:  trace.OutcomeAccepted,
		Pos:      call.Pos(),
		Function: call.Parent().Name(),
		Details:  map[string]string{"method": proof.method, "lock": proof.identity},
	}
	probe.Candidate(step)
	if proof.discard {
		step.Outcome = trace.OutcomeRejected
	}
	probe.Decision(step)
}
