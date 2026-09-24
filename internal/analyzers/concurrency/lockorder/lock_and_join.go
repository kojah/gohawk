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

// These proofs consume one complete parent/sole-worker graph. Fresh resources
// rule out outside unlockers and signallers; opaque participation makes the
// graph unavailable. A cycle is used only after both dependencies are proved.
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
	candidate := potentialLockJoinRoot(function)
	if candidate == token.NoPos {
		return
	}
	joinProbe := analysisTrace.For(pass, "lockorder", string(check.LockAndJoin), candidate)
	joinProbe.Candidate(analysisTrace.Step{Reason: "lock-join-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: candidate})
	root := engine.Root(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget).Observed(joinProbe.Observer()))
	graph := syncgraph.FromSummary(root)
	reportLockSignal(pass, joinProbe, check.LockAndJoin, candidate, proveLockSignal(graph, concurrencyfacts.Close),
		"waits for a worker that needs the held lock")
	channelProbe := analysisTrace.For(pass, "lockorder", string(check.LockChannelCycle), candidate)
	channelProbe.Candidate(analysisTrace.Step{Reason: "channel-lock-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: candidate})
	reportLockSignal(pass, channelProbe, check.LockChannelCycle, candidate, proveLockSignal(graph, concurrencyfacts.Send),
		"receives while holding the lock needed by its sender")
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

func proveLockSignal(graph syncgraph.SyncGraph, signal concurrencyfacts.Kind) lockSignalProof {
	candidate, failure := findLockSignalParent(graph)
	if failure.reason != "" {
		return failure
	}
	return proveLockSignalWorker(graph, candidate, signal)
}

func findLockSignalParent(graph syncgraph.SyncGraph) (lockSignalCandidate, lockSignalProof) {
	if !graph.Complete() {
		return lockSignalCandidate{}, lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: graph.Reason}
	}
	// A fresh allocation excludes an unknown caller or sibling goroutine from
	// releasing the mutex or signalling the channel. Requiring the unlock
	// after the receive makes the parent's side of the cycle explicit.
	if graph.Spawn == nil || graph.Prefix != 1 || len(graph.Parent) < 3 || len(graph.Child) < 2 {
		return lockSignalCandidate{}, lockSignalProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-shape-not-matched"}
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

func proveLockSignalWorker(graph syncgraph.SyncGraph, candidate lockSignalCandidate, signal concurrencyfacts.Kind) lockSignalProof {
	workerLock := graph.Child[0]
	if workerLock.Kind != concurrencyfacts.Lock || workerLock.Resource.Indirect || workerLock.Resource.Value != candidate.mutex {
		return lockSignalProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-worker-order-not-matched"}
	}
	if signal == concurrencyfacts.Send && !unbuffered(candidate.done) {
		return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-lock-capacity-unknown"}
	}
	// The first signal on the waited-for channel determines how the receive
	// can finish. A later close must not establish a lock-and-join defect when
	// an earlier send can unblock the parent, and conversely.
	for _, event := range graph.Child[1:] {
		if event.Resource.Indirect || event.Resource.Value != candidate.done ||
			(event.Kind != concurrencyfacts.Send && event.Kind != concurrencyfacts.Close) {
			continue
		}
		if event.Kind != signal {
			return lockSignalProof{outcome: analysisTrace.OutcomeRejected, reason: "lock-join-signal-kind-not-matched"}
		}
		if !candidate.parentLock.Source.IsValid() || !candidate.wait.Source.IsValid() ||
			!workerLock.Source.IsValid() || !event.Source.IsValid() {
			return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-source-unknown"}
		}
		// The root must finish its receive before unlocking. The worker must
		// acquire that same lock before it can signal. These two dependency
		// edges close a cycle with the graph's program-order edges.
		if !graph.AddDependency(candidate.unlock.ID, workerLock.ID) ||
			!graph.AddDependency(event.ID, candidate.wait.ID) || !graph.HasCycle() {
			return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-cycle-unproven"}
		}
		reason := "lock-join-deadlock-proven"
		if signal == concurrencyfacts.Send {
			reason = "channel-lock-cycle-proven"
		}
		return lockSignalProof{
			parentLock: candidate.parentLock.Source, wait: candidate.wait.Source,
			workerLock: workerLock.Source, signal: event.Source,
			outcome: analysisTrace.OutcomeAccepted, reason: reason,
		}
	}
	return lockSignalProof{outcome: analysisTrace.OutcomeUnknown, reason: "lock-join-completion-unknown"}
}

func unbuffered(channel *ssa.MakeChan) bool {
	size, ok := channel.Size.(*ssa.Const)
	return ok && size.Value != nil && constant.Sign(size.Value) == 0
}
