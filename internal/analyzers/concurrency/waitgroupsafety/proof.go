package waitgroupsafety

// Counter proofs consume complete effects, never count source-level Done calls.
// One worker is admitted only when the caller cannot change its starting count.
import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

type counterProof struct {
	ssaflow.Proof
	operation concurrencyfacts.Operation
}

func proveCounter(function *ssa.Function, summary concurrencyfacts.Summary) counterProof {
	if !summary.Complete() {
		return counterProof{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceReason(summary.Reason)}}
	}
	operations, ordered := counterSequence(summary)
	if !ordered {
		return counterProof{Proof: ssaflow.Proof{Reason: "counter-order-unknown"}}
	}
	counts := make(map[concurrencyfacts.Reference]int)
	for _, operation := range operations {
		allocation, fresh := operation.Resource.Value.(*ssa.Alloc)
		if !fresh || operation.Resource.Indirect || allocation.Parent() != function {
			return counterProof{Proof: ssaflow.Proof{Reason: "counter-initial-state-unknown"}}
		}
		count := counts[operation.Resource]
		switch operation.Kind {
		case concurrencyfacts.GroupAdd:
			counts[operation.Resource] = count + 1
		case concurrencyfacts.GroupDone:
			if count == 0 {
				return counterProof{
					Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "counter-underflow"}, operation: operation,
				}
			}
			counts[operation.Resource] = count - 1
		case concurrencyfacts.GroupWait:
			if count != 0 {
				return counterProof{Proof: ssaflow.Proof{Reason: "counter-wait-blocks"}}
			}
		default:
			return counterProof{Proof: ssaflow.Proof{Reason: "counter-effect-unknown"}}
		}
	}
	return counterProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: "counter-no-underflow"}}
}

func counterSequence(summary concurrencyfacts.Summary) ([]concurrencyfacts.Operation, bool) {
	if summary.Spawn == nil {
		return summary.Operations, true
	}
	// A wait-only caller cannot supply a later Add that rescues the worker.
	// Reject caller decrements too: they would require interleaving analysis.
	for _, operation := range summary.Operations[summary.Prefix:] {
		if operation.Kind != concurrencyfacts.GroupWait {
			return nil, false
		}
	}
	for _, operation := range summary.Worker {
		if operation.Kind != concurrencyfacts.GroupDone {
			return nil, false
		}
	}
	operations := append([]concurrencyfacts.Operation(nil), summary.Operations[:summary.Prefix]...)
	return append(operations, summary.Worker...), true
}
