package lockorder

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This proof consumes the complete parent/sole-worker event fragment. A fresh
// mutex and completion channel rule out outside unlockers and signallers;
// opaque participation or a second launch makes the fragment unavailable.
// The general synchronization graph may have candidate cycles, but this check
// reports only the exact three-event parent and must-lock-before-close worker.
type lockJoinProof struct {
	parentLock token.Pos
	wait       token.Pos
	workerLock token.Pos
	close      token.Pos
	outcome    analysisTrace.Outcome
	reason     string
}

func (proof lockJoinProof) proven() bool { return proof.outcome == analysisTrace.OutcomeAccepted }

type lockJoinCandidate struct {
	mutex      *ssa.Alloc
	done       *ssa.MakeChan
	parentLock token.Pos
	wait       token.Pos
}

func reportLockAndJoin(pass *analysis.Pass, function *ssa.Function, engine *concurrencyfacts.Engine) {
	candidate := potentialLockJoinRoot(function)
	if candidate == token.NoPos {
		return
	}
	probe := analysisTrace.For(pass, "lockorder", string(check.LockAndJoin), candidate)
	probe.Candidate(analysisTrace.Step{Reason: "lock-join-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: candidate})
	root := engine.Root(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget).Observed(probe.Observer()))
	proof := proveLockAndJoin(root)
	probe.Decision(analysisTrace.Step{Reason: proof.reason, Outcome: proof.outcome, Pos: candidate})
	if !proof.proven() {
		return
	}
	source := syntax.SourceRange(pass, proof.wait)
	check.Report(pass, check.LockAndJoin, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(),
		Message: "waits for a worker that needs the held lock",
		Related: []analysis.RelatedInformation{
			{Pos: proof.parentLock, Message: "mutex acquired here"},
			{Pos: proof.workerLock, Message: "worker must acquire the same mutex"},
			{Pos: proof.close, Message: "worker signals completion after acquiring the mutex"},
		},
	})
}

func potentialLockJoinRoot(function *ssa.Function) token.Pos {
	if function == nil || len(function.Blocks) == 0 {
		return token.NoPos
	}
	// This is only a cost filter. The complete root summary, not the presence
	// of these instructions, establishes whether another participant or effect
	// could change the wait. Avoid spending a summary budget on every launch.
	var launches int
	var wait token.Pos
	var localChannel, lock bool
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.Go:
				launches++
			case *ssa.MakeChan:
				localChannel = true
			case *ssa.UnOp:
				if instruction.Op == token.ARROW && wait == token.NoPos {
					wait = instruction.Pos()
				}
			case *ssa.Call:
				effect, known := directMutexEffect(instruction)
				lock = lock || known && effect.operation == mutexAcquire
			}
		}
	}
	if launches != 1 || !localChannel || !lock {
		return token.NoPos
	}
	return wait
}

func proveLockAndJoin(root concurrencyfacts.Summary) lockJoinProof {
	candidate, failure := findLockJoinParent(root)
	if failure.reason != "" {
		return failure
	}
	return proveLockJoinWorker(candidate, root.Worker)
}

func findLockJoinParent(root concurrencyfacts.Summary) (lockJoinCandidate, lockJoinProof) {
	if !root.Complete() {
		return lockJoinCandidate{}, lockJoinProof{outcome: analysisTrace.OutcomeUnknown, reason: root.Reason}
	}
	// Fresh allocations prevent an unknown caller or a sibling goroutine from
	// unlocking the parent's mutex or signalling its channel. Requiring the
	// unlock after the receive makes the circular wait explicit, rather than
	// inferring ownership from a missing release.
	if root.Spawn == nil || root.Prefix != 1 || len(root.Operations) < 3 || len(root.Worker) < 2 {
		return lockJoinCandidate{}, lockJoinProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-shape-not-matched"}
	}
	lock, wait, unlock := root.Operations[0], root.Operations[1], root.Operations[2]
	if !parentLockWaitUnlock(lock, wait, unlock) {
		return lockJoinCandidate{}, lockJoinProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-parent-order-not-matched"}
	}
	mutex, localMutex := lock.Resource.Value.(*ssa.Alloc)
	done, localChannel := wait.Resource.Value.(*ssa.MakeChan)
	if !localMutex || !localChannel || !exactParentResources(lock, wait, unlock, mutex) {
		return lockJoinCandidate{}, lockJoinProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-identity-unknown"}
	}
	return lockJoinCandidate{mutex: mutex, done: done, parentLock: lock.Source, wait: wait.Source}, lockJoinProof{}
}

func parentLockWaitUnlock(lock, wait, unlock concurrencyfacts.Operation) bool {
	return lock.Kind == concurrencyfacts.Lock && wait.Kind == concurrencyfacts.Receive && unlock.Kind == concurrencyfacts.Unlock
}

func exactParentResources(lock, wait, unlock concurrencyfacts.Operation, mutex *ssa.Alloc) bool {
	return !lock.Resource.Indirect && !wait.Resource.Indirect && !unlock.Resource.Indirect && unlock.Resource.Value == mutex
}

func proveLockJoinWorker(candidate lockJoinCandidate, worker []concurrencyfacts.Operation) lockJoinProof {
	workerLock := worker[0]
	// If another synchronization event precedes this lock, the worker may
	// block or signal by another route before reaching it. The first-event
	// requirement is intentionally narrower than general cycle feasibility.
	if workerLock.Kind != concurrencyfacts.Lock || workerLock.Resource.Indirect || workerLock.Resource.Value != candidate.mutex {
		return lockJoinProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-worker-order-not-matched"}
	}
	for _, operation := range worker[1:] {
		if operation.Kind == concurrencyfacts.Close && !operation.Resource.Indirect && operation.Resource.Value == candidate.done {
			if !candidate.parentLock.IsValid() || !candidate.wait.IsValid() ||
				!workerLock.Source.IsValid() || !operation.Source.IsValid() {
				return lockJoinProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-source-unknown"}
			}
			return lockJoinProof{
				parentLock: candidate.parentLock, wait: candidate.wait, workerLock: workerLock.Source, close: operation.Source,
				outcome: analysisTrace.OutcomeAccepted, reason: "lock-join-deadlock-proven",
			}
		}
	}
	return lockJoinProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-completion-unknown"}
}
