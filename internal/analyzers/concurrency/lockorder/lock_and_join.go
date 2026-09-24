package lockorder

import (
	"go/constant"
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syncgraph"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// These proofs consume one complete bounded parent/children graph. Fresh
// resources rule out outside participants, and every known child is checked
// for an alternate unlock or signal. A cycle is used only after both
// dependencies are proved.
type lockSignalProof struct {
	parentLock token.Pos
	wait       token.Pos
	workerLock token.Pos
	signal     token.Pos
	outcome    analysisTrace.Outcome
	reason     string
}

func (proof lockSignalProof) proven() bool { return proof.outcome == analysisTrace.OutcomeAccepted }

type lockSignalCandidate struct {
	mutex      *ssa.Alloc
	done       *ssa.MakeChan
	parentLock syncgraph.SyncEvent
	wait       syncgraph.SyncEvent
	unlock     syncgraph.SyncEvent
}

func reportSynchronizationCycles(pass *analysis.Pass, function *ssa.Function, engine *concurrencyfacts.Engine) {
	channelCandidate := potentialLockJoinRoot(function)
	groupCandidate := potentialWaitGroupLockRoot(function)
	if channelCandidate == token.NoPos && groupCandidate == token.NoPos {
		return
	}
	candidate := channelCandidate
	probeID := check.LockAndJoin
	if candidate == token.NoPos {
		candidate = groupCandidate
		probeID = check.LockWaitGroupCycle
	}
	probe := analysisTrace.For(pass, "lockorder", string(probeID), candidate)
	probe.Candidate(analysisTrace.Step{Reason: "sync-cycle-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: candidate})
	root := engine.Root(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget).Observed(probe.Observer()))
	graph := syncgraph.FromSummary(root)
	if channelCandidate != token.NoPos {
		joinProbe := analysisTrace.For(pass, "lockorder", string(check.LockAndJoin), channelCandidate)
		joinProbe.Candidate(analysisTrace.Step{Reason: "lock-join-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: channelCandidate})
		reportLockSignal(pass, joinProbe, check.LockAndJoin, channelCandidate, proveLockSignal(graph, concurrencyfacts.Close),
			"waits for a worker that needs the held lock")
		channelProbe := analysisTrace.For(pass, "lockorder", string(check.LockChannelCycle), channelCandidate)
		channelProbe.Candidate(analysisTrace.Step{Reason: "channel-lock-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: channelCandidate})
		reportLockSignal(pass, channelProbe, check.LockChannelCycle, channelCandidate, proveLockSignal(graph, concurrencyfacts.Send),
			"receives while holding the lock needed by its sender")
	}
	if groupCandidate != token.NoPos {
		reportWaitGroupLockCycle(pass, graph, groupCandidate)
	}
}

func reportLockSignal(
	pass *analysis.Pass, probe analysisTrace.Probe, id check.ID, candidate token.Pos, proof lockSignalProof, message string,
) {
	probe.Decision(analysisTrace.Step{Reason: proof.reason, Outcome: proof.outcome, Pos: candidate})
	if !proof.proven() {
		return
	}
	source := syntax.SourceRange(pass, proof.wait)
	check.Report(pass, id, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(), Message: message,
		Related: []analysis.RelatedInformation{
			{Pos: proof.parentLock, Message: "mutex acquired here"},
			{Pos: proof.workerLock, Message: "worker must acquire the same mutex"},
			{Pos: proof.signal, Message: "worker signals only after acquiring the mutex"},
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
				if instruction.Common().StaticCallee() != nil {
					launches++ // A complete callee summary decides whether it actually launches.
				}
				effect, known := directMutexEffect(instruction)
				lock = lock || known && effect.operation == mutexAcquire
			}
		}
	}
	if launches == 0 || !localChannel || !lock {
		return token.NoPos
	}
	return wait
}

func proveLockSignal(graph syncgraph.SyncGraph, signal concurrencyfacts.Kind) lockSignalProof {
	candidate, failure := findLockSignalParent(graph)
	if failure.reason != "" {
		return failure
	}
	return proveLockSignalChildren(graph, candidate, signal)
}

func findLockSignalParent(graph syncgraph.SyncGraph) (lockSignalCandidate, lockSignalProof) {
	if !graph.Complete() {
		return lockSignalCandidate{}, lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: graph.Reason}
	}
	// Fresh local resources exclude unknown callers. Every recorded child is
	// checked below for an alternative unlock or signal. Requiring the parent's
	// unlock after the receive makes its side of the cycle explicit.
	if len(graph.Children) == 0 || len(graph.Parent) < 3 {
		return lockSignalCandidate{}, lockSignalProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-shape-not-matched"}
	}
	for _, child := range graph.Children {
		if !child.LaunchKnown() || child.Prefix != 1 {
			return lockSignalCandidate{}, lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-spawn-order-unknown"}
		}
	}
	lock, wait, unlock := graph.Parent[0], graph.Parent[1], graph.Parent[2]
	if lock.Kind != concurrencyfacts.Lock || wait.Kind != concurrencyfacts.Receive || unlock.Kind != concurrencyfacts.Unlock {
		return lockSignalCandidate{}, lockSignalProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-parent-order-not-matched"}
	}
	mutex, localMutex := lock.Resource.Value.(*ssa.Alloc)
	done, localChannel := wait.Resource.Value.(*ssa.MakeChan)
	if !localMutex || !localChannel || lock.Resource.Indirect || wait.Resource.Indirect || unlock.Resource.Indirect ||
		unlock.Resource.Value != mutex {
		return lockSignalCandidate{}, lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-identity-unknown"}
	}
	return lockSignalCandidate{mutex: mutex, done: done, parentLock: lock, wait: wait, unlock: unlock}, lockSignalProof{}
}

func proveLockSignalChildren(graph syncgraph.SyncGraph, candidate lockSignalCandidate, signal concurrencyfacts.Kind) lockSignalProof {
	if signal == concurrencyfacts.Send && !unbuffered(candidate.done) {
		return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-lock-capacity-unknown"}
	}
	var witness workerSignal
	for _, child := range graph.Children {
		found, failure := classifyWorkerSignal(child, candidate, signal)
		if failure.reason != "" {
			return failure
		}
		if found.present && !witness.present {
			witness = found
		}
	}
	if !witness.present {
		return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-completion-unknown"}
	}
	if !candidate.parentLock.Source.IsValid() || !candidate.wait.Source.IsValid() ||
		!witness.lock.Source.IsValid() || !witness.signal.Source.IsValid() {
		return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-source-unknown"}
	}
	// Every other child has now been checked for an earlier signal or unlock.
	// The witness cannot signal until it acquires the mutex, and the parent
	// cannot unlock until this receive finishes. The proven edges close a cycle.
	if !graph.AddDependency(candidate.unlock.ID, witness.lock.ID) ||
		!graph.AddDependency(witness.signal.ID, candidate.wait.ID) || !graph.HasCycle() {
		return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-cycle-unproven"}
	}
	reason := "lock-join-deadlock-proven"
	if signal == concurrencyfacts.Send {
		reason = "channel-lock-cycle-proven"
	}
	return lockSignalProof{
		parentLock: candidate.parentLock.Source, wait: candidate.wait.Source,
		workerLock: witness.lock.Source, signal: witness.signal.Source,
		outcome: analysisTrace.OutcomeAccepted, reason: reason,
	}
}

type workerSignal struct {
	lock    syncgraph.SyncEvent
	signal  syncgraph.SyncEvent
	present bool
}

func classifyWorkerSignal(
	child syncgraph.SyncChild, candidate lockSignalCandidate, signal concurrencyfacts.Kind,
) (workerSignal, lockSignalProof) {
	var lockedBehindParent syncgraph.SyncEvent
	locked := false
	for _, event := range child.Events {
		if event.Resource.Indirect {
			return workerSignal{}, lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-worker-identity-unknown"}
		}
		// Cond.Wait unlocks its associated Locker while waiting. Until this
		// child is itself blocked on the parent's mutex, an unmodeled locker
		// relationship could let it release the parent's hold.
		if event.Kind == concurrencyfacts.CondWait && !locked {
			return workerSignal{}, lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-alternate-unlock"}
		}
		if event.Resource.Value == candidate.mutex {
			if event.Kind == concurrencyfacts.Unlock && !locked {
				return workerSignal{}, lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-alternate-unlock"}
			}
			if event.Kind == concurrencyfacts.Lock && !locked {
				lockedBehindParent = event
				locked = true
			}
		}
		if event.Resource.Value != candidate.done || event.Kind != concurrencyfacts.Send && event.Kind != concurrencyfacts.Close {
			continue
		}
		// The first signal from each child is the one that could complete the
		// parent's receive. A later signal never repairs an earlier alternative.
		if event.Kind != signal {
			return workerSignal{}, lockSignalProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-signal-kind-not-matched"}
		}
		if !locked {
			return workerSignal{}, lockSignalProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-worker-order-not-matched"}
		}
		return workerSignal{lock: lockedBehindParent, signal: event, present: true}, lockSignalProof{}
	}
	return workerSignal{}, lockSignalProof{}
}

func unbuffered(channel *ssa.MakeChan) bool {
	size, ok := channel.Size.(*ssa.Const)
	return ok && size.Value != nil && constant.Sign(size.Value) == 0
}
