package concurrencyfacts

import (
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Function summaries retain child templates; root queries instantiate each
// call as a distinct child. A nested launch in a worker body remains unknown.

// maxWorkers bounds the number of separately ordered child sequences in a
// root query. Launches in loops remain unknown regardless of this limit.
const maxWorkers = 4

func (engine *Engine) appendGo(result *Summary, instruction *ssa.Go) string {
	// A function records each statically known launch separately. A fifth
	// participant or a child with unavailable effects makes the *whole*
	// protocol unknown: otherwise a missing alternate signal or unlock
	// could turn a feasible wait into a false deadlock proof.
	if len(result.Workers) >= maxWorkers {
		return "protocol-participants-unknown"
	}
	called := engine.instantiate(instruction)
	result.CancellationInputs = append(result.CancellationInputs, called.CancellationInputs...)
	if !composableLinear(called) {
		if called.Reason != "protocol-select-alternatives" || len(called.Choices) != 1 || !completeChoice(called.Choices[0]) {
			return called.Reason
		}
		worker := WorkerSummary{Spawn: instruction, Site: instruction.Pos(), Prefix: len(result.Operations)}
		choice := called.Choices[0]
		choice.Worker = instruction
		result.Choices = append(result.Choices, choice)
		for _, arm := range choice.Arms {
			worker.Alternatives = append(worker.Alternatives, arm.Sequence)
		}
		result.Workers = append(result.Workers, worker)
		return ""
	}
	if len(called.Workers) != 0 {
		return "protocol-worker-effects-unknown"
	}
	result.Workers = append(result.Workers, WorkerSummary{
		Operations: called.Operations, Spawn: instruction, Site: instruction.Pos(), Prefix: len(result.Operations),
	})
	return ""
}

func (engine *Engine) appendCall(result *Summary, instruction *ssa.Call) string {
	called := engine.callSummary(instruction)
	result.CancellationInputs = append(result.CancellationInputs, called.CancellationInputs...)
	if len(called.Workers) != 0 {
		if !composableLinear(called) || len(result.Workers)+len(called.Workers) > maxWorkers {
			return "protocol-participants-unknown"
		}
		for _, worker := range called.Workers {
			if len(worker.Alternatives) != 0 {
				return "protocol-worker-alternatives-unknown"
			}
			worker.Prefix += len(result.Operations)
			worker.Site = instruction.Pos()
			result.Workers = append(result.Workers, worker)
		}
	}
	for _, choice := range called.Choices {
		choice.Prefix += len(result.Operations)
		result.Choices = append(result.Choices, choice)
	}
	result.Operations = append(result.Operations, called.Operations...)
	if called.Reason == cancellationBindingRequired {
		return ""
	}
	return called.Reason
}

func (engine *Engine) bindWorker(
	worker WorkerSummary, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction,
) (WorkerSummary, string) {
	if len(worker.Alternatives) != 0 {
		return WorkerSummary{}, "protocol-worker-alternatives-unknown"
	}
	bound := WorkerSummary{Spawn: worker.Spawn, Site: instruction.Pos(), Prefix: worker.Prefix}
	for _, op := range worker.Operations {
		if !engine.budget.Spend() {
			return WorkerSummary{}, "protocol-budget-exhausted"
		}
		resource, ok := engine.bind(op.Resource, bindings, instruction)
		if !ok {
			return WorkerSummary{}, "protocol-channel-binding-unknown"
		}
		op.Resource, op.Site = resource, instruction.Pos()
		bound.Operations = append(bound.Operations, op)
	}
	return bound, ""
}

func (summary Summary) hasWorkerAlternatives() bool {
	for _, worker := range summary.Workers {
		if len(worker.Alternatives) != 0 {
			return true
		}
	}
	return false
}
