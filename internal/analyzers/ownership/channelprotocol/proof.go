package channelprotocol

import (
	"go/constant"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// The first proof joins two ordered summaries, not two execution state spaces.
// Both channels are fresh root allocations and every effect is accounted for.
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
	if protocol.spawn == nil || protocol.prefix != 0 || len(protocol.operations) != 2 || len(protocol.worker) != 2 {
		return unknown
	}
	wait, receive := protocol.operations[0], protocol.operations[1]
	send, signal := protocol.worker[0], protocol.worker[1]
	if wait.kind != receiveOperation || receive.kind != receiveOperation || send.kind != sendOperation || signal.kind != closeOperation {
		return unknown
	}
	if wait.channel != signal.channel || receive.channel != send.channel || wait.channel == send.channel {
		return unknown
	}
	resultCapacity, resultLocal := localCapacity(function, send.channel)
	_, doneLocal := localCapacity(function, wait.channel)
	if !resultLocal || !doneLocal {
		return cycleProof{Proof: ssaflow.Proof{Reason: "protocol-channel-scope-unknown"}}
	}
	if resultCapacity > 0 {
		return cycleProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: "protocol-buffer-allows-progress"}}
	}
	return cycleProof{
		Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "protocol-wait-cycle", Provenance: ssaflow.EvidenceFromLocalSSA},
		wait:  wait, send: send, signal: signal, receive: receive,
	}
}

func localCapacity(function *ssa.Function, reference channelReference) (int64, bool) {
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
