package channelprotocol

import (
	"go/constant"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// The first proof joins two ordered summaries, not two execution state spaces.
// Both resources are fresh root allocations and every effect is accounted for.
// Thus nobody else can receive the result or signal completion to break the cycle.
type cycleProof struct {
	ssaflow.Proof
	wait, send, signal, receive operation
}

func (engine *summaryEngine) prove(function *ssa.Function, limit int) cycleProof {
	engine.begin(limit)
	protocol := engine.collect(function, true)
	if engine.budget.Exhausted() {
		return cycleProof{Proof: ssaflow.Proof{Reason: "protocol-budget-exhausted"}}
	}
	if protocol.reason != "" {
		return cycleProof{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceReason(protocol.reason)}}
	}
	return proveCycle(function, protocol)
}

func proveCycle(function *ssa.Function, protocol summary) cycleProof {
	unknown := cycleProof{Proof: ssaflow.Proof{Reason: "protocol-order-not-matched"}}
	if protocol.spawn == nil || protocol.prefix > 1 || len(protocol.operations) != protocol.prefix+2 || len(protocol.worker) != 2 {
		return unknown
	}
	wait, receive := protocol.operations[protocol.prefix], protocol.operations[protocol.prefix+1]
	send, signal := protocol.worker[0], protocol.worker[1]
	if receive.kind != receiveOperation || send.kind != sendOperation {
		return unknown
	}
	if wait.resource != signal.resource || receive.resource != send.resource || wait.resource == send.resource {
		return unknown
	}
	resultCapacity, resultLocal := localCapacity(function, send.resource)
	if !resultLocal {
		return cycleProof{Proof: ssaflow.Proof{Reason: "protocol-channel-scope-unknown"}}
	}
	if completion := proveCompletionScope(function, protocol, wait, signal); !completion.Proven() {
		return cycleProof{Proof: completion}
	}
	if resultCapacity > 0 {
		return cycleProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: "protocol-buffer-allows-progress"}}
	}
	return cycleProof{
		Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "protocol-wait-cycle", Provenance: ssaflow.EvidenceFromLocalSSA},
		wait:  wait, send: send, signal: signal, receive: receive,
	}
}

func localCapacity(function *ssa.Function, reference resourceReference) (int64, bool) {
	created, ok := reference.value.(*ssa.MakeChan)
	if !ok || reference.indirect || created.Parent() != function {
		return 0, false
	}
	size, ok := created.Size.(*ssa.Const)
	if !ok || size.Value == nil {
		return 0, false
	}
	capacity, exact := constant.Int64Val(size.Value)
	return capacity, exact && capacity >= 0
}

func proveCompletionScope(function *ssa.Function, protocol summary, wait, signal operation) ssaflow.Proof {
	proven := ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "protocol-local-completion", Provenance: ssaflow.EvidenceFromLocalSSA}
	if wait.kind == receiveOperation && signal.kind == closeOperation && protocol.prefix == 0 {
		_, local := localCapacity(function, wait.resource)
		if local {
			return proven
		}
		return ssaflow.Proof{Reason: "protocol-channel-scope-unknown"}
	}
	if wait.kind != groupWaitOperation || signal.kind != groupDoneOperation || protocol.prefix != 1 {
		return ssaflow.Proof{Reason: "protocol-completion-order-unknown"}
	}
	add := protocol.operations[0]
	if add.kind != groupAddOperation || add.resource != wait.resource || wait.resource.indirect {
		return ssaflow.Proof{Reason: "protocol-group-obligation-unknown"}
	}
	// The collector admits exactly Add(1), forbids group resets and accounts
	// for every participant. A fresh group's sole outstanding count therefore
	// cannot reach zero until this worker gets past the blocked send.
	created, ok := wait.resource.value.(*ssa.Alloc)
	if !ok || created.Parent() != function || !waitGroupPointer(created.Type()) {
		return ssaflow.Proof{Reason: "protocol-group-scope-unknown"}
	}
	return proven
}
