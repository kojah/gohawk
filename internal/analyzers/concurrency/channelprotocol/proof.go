package channelprotocol

import (
	"go/constant"

	"github.com/kojah/gohawk/internal/check"
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

func (engine *summaryEngine) proveCheck(function *ssa.Function, limit int, id check.ID) cycleProof {
	budget := ssaflow.NewSearchBudget(limit)
	protocol := engine.Root(function, budget)
	if budget.Exhausted() {
		return cycleProof{Proof: ssaflow.Proof{Reason: "protocol-budget-exhausted"}}
	}
	if !protocol.Complete() {
		return cycleProof{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceReason(protocol.Reason)}}
	}
	if id == check.ChannelProtocolBlocked {
		return proveCycle(function, protocol)
	}
	return proveMixedCycle(function, protocol, id)
}

func proveCycle(function *ssa.Function, protocol summary) cycleProof {
	unknown := cycleProof{Proof: ssaflow.Proof{Reason: "protocol-order-not-matched"}}
	if protocol.Spawn == nil || protocol.Prefix > 1 || len(protocol.Operations) != protocol.Prefix+2 || len(protocol.Worker) != 2 {
		return unknown
	}
	wait, receive := protocol.Operations[protocol.Prefix], protocol.Operations[protocol.Prefix+1]
	send, signal := protocol.Worker[0], protocol.Worker[1]
	if receive.Kind != receiveOperation || send.Kind != sendOperation {
		return unknown
	}
	if wait.Resource != signal.Resource || receive.Resource != send.Resource || wait.Resource == send.Resource {
		return unknown
	}
	resultCapacity, resultLocal := localCapacity(function, send.Resource)
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
	created, ok := reference.Value.(*ssa.MakeChan)
	if !ok || reference.Indirect || created.Parent() != function {
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
	if wait.Kind == receiveOperation && signal.Kind == closeOperation && protocol.Prefix == 0 {
		_, local := localCapacity(function, wait.Resource)
		if local {
			return proven
		}
		return ssaflow.Proof{Reason: "protocol-channel-scope-unknown"}
	}
	if wait.Kind != groupWaitOperation || signal.Kind != groupDoneOperation || protocol.Prefix != 1 {
		return ssaflow.Proof{Reason: "protocol-completion-order-unknown"}
	}
	add := protocol.Operations[0]
	if add.Kind != groupAddOperation || add.Resource != wait.Resource || wait.Resource.Indirect {
		return ssaflow.Proof{Reason: "protocol-group-obligation-unknown"}
	}
	// The collector admits exactly Add(1), forbids group resets and accounts
	// for every participant. A fresh group's sole outstanding count therefore
	// cannot reach zero until this worker gets past the blocked send.
	created, ok := wait.Resource.Value.(*ssa.Alloc)
	if !ok || created.Parent() != function || !waitGroupPointer(created.Type()) {
		return ssaflow.Proof{Reason: "protocol-group-scope-unknown"}
	}
	return proven
}
