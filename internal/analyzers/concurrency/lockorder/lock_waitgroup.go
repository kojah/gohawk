package lockorder

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syncmodel"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This proof owns the exact WaitGroup counter contract. A cycle alone is not
// enough: each Add(1) must correspond to one known child, and every child must
// reach Done only after acquiring the parent's fresh, still-held mutex.
var waitGroupWait = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Wait"})

type waitGroupCycleProof struct {
	wait       token.Pos
	parentLock token.Pos
	workerLock token.Pos
	done       token.Pos
	outcome    analysisTrace.Outcome
	reason     string
}

type waitGroupParent struct {
	group  ssa.Value
	mutex  ssa.Value
	lock   syncmodel.SyncEvent
	wait   syncmodel.SyncEvent
	unlock syncmodel.SyncEvent
}

func potentialWaitGroupLockRoot(function *ssa.Function) token.Pos {
	if function == nil {
		return token.NoPos
	}
	var launch, lock bool
	var wait token.Pos
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.Go:
				launch = true
			case *ssa.Call:
				if instruction.Common().StaticCallee() != nil {
					launch = true // A complete callee summary decides whether it actually launches.
				}
				if effect, known := directMutexEffect(instruction); known && effect.operation == mutexAcquire {
					lock = true
				}
				if wait == token.NoPos && ssaflow.CallMatchesSymbol(instruction.Common(), waitGroupWait) {
					wait = instruction.Pos()
				}
			}
		}
	}
	if !launch || !lock {
		return token.NoPos
	}
	return wait
}

func reportWaitGroupLockCycle(pass *analysis.Pass, graphs []syncmodel.SyncGraph, reason string, candidate token.Pos) {
	probe := analysisTrace.For(pass, "lockorder", string(check.LockWaitGroupCycle), candidate)
	probe.Candidate(analysisTrace.Step{Reason: "waitgroup-lock-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: candidate})
	proof := proveWaitGroupVariants(graphs, reason)
	probe.Decision(analysisTrace.Step{Reason: proof.reason, Outcome: proof.outcome, Pos: candidate})
	if proof.outcome != analysisTrace.OutcomeAccepted {
		return
	}
	source := syntax.SourceRange(pass, proof.wait)
	check.Report(pass, check.LockWaitGroupCycle, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(), Message: "waits for counted workers that need the held lock",
		Related: []analysis.RelatedInformation{
			{Pos: proof.parentLock, Message: "mutex acquired here"},
			{Pos: proof.workerLock, Message: "counted worker must acquire this mutex"},
			{Pos: proof.done, Message: "worker can decrement the group only after acquiring it"},
		},
	})
}

func proveWaitGroupVariants(graphs []syncmodel.SyncGraph, reason string) waitGroupCycleProof {
	if reason != "" || len(graphs) == 0 {
		return waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: reason}
	}
	var common waitGroupCycleProof
	for _, graph := range graphs {
		proof := proveWaitGroupLockCycle(graph)
		if proof.outcome != analysisTrace.OutcomeAccepted {
			return proof
		}
		if common.reason != "" && (common.wait != proof.wait || common.parentLock != proof.parentLock) {
			return waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "waitgroup-lock-alternative-sites-differ"}
		}
		common = proof
	}
	return common
}

func proveWaitGroupLockCycle(graph syncmodel.SyncGraph) waitGroupCycleProof {
	if !graph.Complete() {
		return waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: graph.Reason}
	}
	var failure waitGroupCycleProof
	for _, wait := range graph.Parent {
		if wait.Kind != concurrencyfacts.GroupWait {
			continue
		}
		for _, lock := range graph.Parent {
			if lock.Kind != concurrencyfacts.Lock {
				continue
			}
			proof := proveScopedWaitGroupLockCycle(graph.Scope(wait.Resource, lock.Resource))
			if proof.outcome == analysisTrace.OutcomeAccepted {
				return proof
			}
			failure = proof
		}
	}
	if failure.reason == "" {
		failure = waitGroupCycleProof{outcome: analysisTrace.OutcomeRejected, reason: "waitgroup-lock-shape-not-matched"}
	}
	return failure
}

func proveScopedWaitGroupLockCycle(graph syncmodel.SyncGraph) waitGroupCycleProof {
	if !graph.Complete() {
		return waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: graph.Reason}
	}
	parent, failure := findWaitGroupParent(graph)
	if failure.reason != "" {
		return failure
	}
	witnessLock, witnessDone, failure := findCountedWorkers(graph.Children, parent)
	if failure.reason != "" {
		return failure
	}
	if !parent.lock.Source.IsValid() || !parent.wait.Source.IsValid() ||
		!witnessLock.Source.IsValid() || !witnessDone.Source.IsValid() {
		return waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "waitgroup-lock-source-unknown"}
	}
	// With no other group participant or decrement, the Wait cannot return
	// until a worker reaches Done. Each worker is blocked by the parent's
	// lock, which the parent releases only after Wait returns.
	if !graph.AddDependency(parent.unlock.ID, witnessLock.ID) ||
		!graph.AddDependency(witnessDone.ID, parent.wait.ID) || !graph.HasCycle() {
		return waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "waitgroup-lock-cycle-unproven"}
	}
	return waitGroupCycleProof{
		wait: parent.wait.Source, parentLock: parent.lock.Source, workerLock: witnessLock.Source, done: witnessDone.Source,
		outcome: analysisTrace.OutcomeAccepted, reason: "waitgroup-lock-cycle-proven",
	}
}

func findWaitGroupParent(graph syncmodel.SyncGraph) (waitGroupParent, waitGroupCycleProof) {
	count := len(graph.Children)
	if count == 0 || len(graph.Parent) != count+3 {
		return waitGroupParent{}, waitGroupCycleProof{outcome: analysisTrace.OutcomeRejected, reason: "waitgroup-lock-shape-not-matched"}
	}
	parent := graph.Parent
	group := parent[0].Resource.Value
	if !syncmodel.FreshResource(parent[0].Resource).Proven() || !exactWaitGroupEvent(parent[0], concurrencyfacts.GroupAdd, group) {
		return waitGroupParent{}, waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "waitgroup-lock-counter-unknown"}
	}
	for _, event := range parent[:count] {
		if !exactWaitGroupEvent(event, concurrencyfacts.GroupAdd, group) {
			return waitGroupParent{}, waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "waitgroup-lock-counter-unknown"}
		}
	}
	lock, wait, unlock := parent[count], parent[count+1], parent[count+2]
	mutex := lock.Resource.Value
	if !syncmodel.FreshResource(lock.Resource).Proven() || !exactWaitGroupEvent(lock, concurrencyfacts.Lock, mutex) ||
		!exactWaitGroupEvent(wait, concurrencyfacts.GroupWait, group) ||
		!exactWaitGroupEvent(unlock, concurrencyfacts.Unlock, mutex) {
		return waitGroupParent{}, waitGroupCycleProof{outcome: analysisTrace.OutcomeRejected, reason: "waitgroup-lock-parent-order-not-matched"}
	}
	return waitGroupParent{group: group, mutex: mutex, lock: lock, wait: wait, unlock: unlock}, waitGroupCycleProof{}
}

func findCountedWorkers(
	children []syncmodel.SyncChild, parent waitGroupParent,
) (syncmodel.SyncEvent, syncmodel.SyncEvent, waitGroupCycleProof) {
	var witnessLock, witnessDone syncmodel.SyncEvent
	for index, child := range children {
		if !child.LaunchKnown() || child.Prefix != len(children)+1 || len(child.Events) != 3 {
			return syncmodel.SyncEvent{}, syncmodel.SyncEvent{},
				waitGroupCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "waitgroup-lock-worker-effects-unknown"}
		}
		workerLock, done, workerUnlock := child.Events[0], child.Events[1], child.Events[2]
		if !exactWaitGroupEvent(workerLock, concurrencyfacts.Lock, parent.mutex) ||
			!exactWaitGroupEvent(done, concurrencyfacts.GroupDone, parent.group) ||
			!exactWaitGroupEvent(workerUnlock, concurrencyfacts.Unlock, parent.mutex) {
			return syncmodel.SyncEvent{}, syncmodel.SyncEvent{},
				waitGroupCycleProof{outcome: analysisTrace.OutcomeRejected, reason: "waitgroup-lock-worker-order-not-matched"}
		}
		if index == 0 {
			witnessLock, witnessDone = workerLock, done
		}
	}
	return witnessLock, witnessDone, waitGroupCycleProof{}
}

func exactWaitGroupEvent(event syncmodel.SyncEvent, kind concurrencyfacts.Kind, resource ssa.Value) bool {
	return event.Kind == kind && !event.Resource.Indirect && event.Resource.Value == resource
}
