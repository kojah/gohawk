package condsafety

// Only fresh mutexes in a complete, single-goroutine scope establish an initial
// unlocked state. An earlier blocking or invalid operation ends the proof.
import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

type waitProof struct {
	ssaflow.Proof
	wait  concurrencyfacts.Operation
	mutex concurrencyfacts.Reference
}

func proveWait(function *ssa.Function, engine *concurrencyfacts.Engine) waitProof {
	budget := ssaflow.NewSearchBudget(2000)
	summary := engine.Root(function, budget)
	if summary.Reason != "" {
		return waitProof{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceReason(summary.Reason)}}
	}
	if summary.Spawn != nil {
		return waitProof{Proof: ssaflow.Proof{Reason: "cond-participants-unknown"}}
	}
	held := make(map[concurrencyfacts.Reference]bool)
	for _, operation := range summary.Operations {
		if operation.Kind == concurrencyfacts.CondWait {
			return proveCondition(function, engine, operation, held, budget)
		}
		if !concurrencyfacts.FreshMutex(function, operation.Resource) {
			return waitProof{Proof: ssaflow.Proof{Reason: "cond-mutex-state-unknown"}}
		}
		switch operation.Kind {
		case concurrencyfacts.Lock:
			if held[operation.Resource] {
				return waitProof{Proof: ssaflow.Proof{Reason: "cond-prior-lock-blocks"}}
			}
			held[operation.Resource] = true
		case concurrencyfacts.Unlock:
			if !held[operation.Resource] {
				return waitProof{Proof: ssaflow.Proof{Reason: "cond-prior-unlock-invalid"}}
			}
			held[operation.Resource] = false
		default:
			return waitProof{Proof: ssaflow.Proof{Reason: "cond-effect-unknown"}}
		}
	}
	return waitProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: "cond-no-wait"}}
}

func proveCondition(function *ssa.Function, engine *concurrencyfacts.Engine, operation concurrencyfacts.Operation,
	held map[concurrencyfacts.Reference]bool, budget *ssaflow.SearchBudget,
) waitProof {
	mutex, exact := engine.CondMutex(operation.Resource, budget)
	if !exact || !concurrencyfacts.FreshMutex(function, mutex) || budget.Exhausted() {
		return waitProof{Proof: ssaflow.Proof{Reason: "cond-locker-unknown"}}
	}
	if held[mutex] {
		// Wait releases then reacquires L, but may never return. Do not use
		// subsequent operations as reachable evidence without a signal proof.
		return waitProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: "cond-wait-held"}}
	}
	return waitProof{
		Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "cond-wait-unlocked"}, wait: operation, mutex: mutex,
	}
}
