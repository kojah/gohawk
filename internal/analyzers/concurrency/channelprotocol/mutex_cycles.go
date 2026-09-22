package channelprotocol

// Mixed-cycle proofs require a fresh exclusive mutex held before the sole
// worker starts. Complete ordered effects establish both sides of the wait;
// a lock and a wait merely appearing in the same function are insufficient.

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func proveMixedCycle(function *ssa.Function, protocol summary, id check.ID) cycleProof {
	unknown := cycleProof{Proof: ssaflow.Proof{Reason: "mixed-order-not-matched"}}
	if protocol.Spawn == nil || protocol.Prefix < 1 || protocol.Prefix > 2 ||
		len(protocol.Operations) != protocol.Prefix+2 {
		return unknown
	}
	lock, prefix, ok := mixedPrefix(protocol)
	if !ok {
		return unknown
	}
	protocol.Worker = mixedWorkerSuffix(function, protocol.Worker, lock.Resource)
	if len(protocol.Worker) != 3 {
		return unknown
	}
	wait, unlock := protocol.Operations[protocol.Prefix], protocol.Operations[protocol.Prefix+1]
	workerLock := protocol.Worker[0]
	if lock.Kind != concurrencyfacts.Lock || workerLock.Kind != concurrencyfacts.Lock ||
		unlock.Kind != concurrencyfacts.Unlock || lock.Resource != workerLock.Resource || lock.Resource != unlock.Resource {
		return unknown
	}
	if !localMutex(function, lock.Resource) {
		return cycleProof{Proof: ssaflow.Proof{Reason: "mixed-mutex-scope-unknown"}}
	}
	signal, workerUnlock := protocol.Worker[1], protocol.Worker[2]
	if signal.Kind == concurrencyfacts.Unlock {
		signal, workerUnlock = workerUnlock, signal
	}
	if workerUnlock.Kind != concurrencyfacts.Unlock || workerUnlock.Resource != lock.Resource {
		return unknown
	}
	proof := mixedDependency(function, prefix, wait, signal, id)
	return cycleProof{Proof: proof, wait: wait, send: workerLock, signal: signal, receive: unlock}
}

// A complete worker may first acquire and release an unrelated fresh mutex.
// Remove only adjacent balanced prefixes: never the selected mutex, a shared
// mutex, an early signal, or a release of the lock held by the parent. The
// remaining three-event proof retains its necessary wait dependency.
func mixedWorkerSuffix(function *ssa.Function, worker []operation, held resourceReference) []operation {
	for len(worker) > 3 {
		acquire, release := worker[0], worker[1]
		if acquire.Kind != concurrencyfacts.Lock || release.Kind != concurrencyfacts.Unlock ||
			acquire.Resource != release.Resource || acquire.Resource == held || !localMutex(function, acquire.Resource) {
			break
		}
		worker = worker[2:]
	}
	return worker
}

func mixedPrefix(protocol summary) (operation, summary, bool) {
	lock := protocol.Operations[0]
	completion := summary{}
	if protocol.Prefix == 2 {
		add := protocol.Operations[1]
		if lock.Kind == groupAddOperation {
			lock, add = add, lock
		}
		if add.Kind != groupAddOperation {
			return operation{}, completion, false
		}
		completion.Prefix, completion.Operations = 1, []operation{add}
	}
	return lock, completion, true
}

func localMutex(function *ssa.Function, reference resourceReference) bool {
	return concurrencyfacts.FreshMutex(function, reference)
}

func mixedDependency(function *ssa.Function, prefix summary, wait, signal operation, id check.ID) ssaflow.Proof {
	if wait.Resource != signal.Resource {
		return ssaflow.Proof{Reason: "mixed-resource-mismatch"}
	}
	if id == check.ChannelProtocolLockJoin {
		proof := proveCompletionScope(function, prefix, wait, signal)
		if proof.Proven() {
			proof.Reason = "mutex-join-cycle"
		}
		return proof
	}
	if prefix.Prefix != 0 || !oppositeCommunication(wait, signal) {
		return ssaflow.Proof{Reason: "mixed-order-not-matched"}
	}
	capacity, local := localCapacity(function, wait.Resource)
	if !local {
		return ssaflow.Proof{Reason: "protocol-channel-scope-unknown"}
	}
	// A buffer might let one side proceed; no occupancy reasoning is attempted.
	if capacity != 0 {
		return ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: "protocol-buffer-allows-progress"}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "mutex-channel-cycle", Provenance: ssaflow.EvidenceFromLocalSSA}
}

func oppositeCommunication(first, second operation) bool {
	return first.Kind == sendOperation && second.Kind == receiveOperation ||
		first.Kind == receiveOperation && second.Kind == sendOperation
}
