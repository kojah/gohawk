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
	if len(called.Paths) != 0 {
		worker := WorkerSummary{Spawn: instruction, Site: instruction.Pos(), Prefix: len(result.Operations), Branches: true}
		for _, path := range called.Paths {
			if !composableLinear(path) || len(path.Workers) != 0 || len(path.Paths) != 0 {
				return "protocol-worker-effects-unknown"
			}
			requireCancellation(result, path.CancellationInputs)
			worker.Alternatives = append(worker.Alternatives, path.Operations)
		}
		result.Workers = append(result.Workers, worker)
		return ""
	}
	if len(called.Workers) != 0 {
		return "protocol-worker-effects-unknown"
	}
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
	result.Workers = append(result.Workers, WorkerSummary{
		Operations: called.Operations, Spawn: instruction, Site: instruction.Pos(), Prefix: len(result.Operations),
	})
	return ""
}

func (engine *Engine) appendCall(result *Summary, instruction *ssa.Call) string {
	called := engine.callSummary(instruction)
	return appendCalled(result, called, instruction)
}

func appendCalled(result *Summary, called Summary, instruction *ssa.Call) string {
	result.CancellationInputs = append(result.CancellationInputs, called.CancellationInputs...)
	if len(called.Workers) != 0 {
		if !composableLinear(called) && !called.workerAlternativesOnly() || len(result.Workers)+len(called.Workers) > maxWorkers {
			return "protocol-participants-unknown"
		}
		for _, worker := range called.Workers {
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
	if called.workerAlternativesOnly() {
		return ""
	}
	if called.Reason == cancellationBindingRequired {
		return ""
	}
	return called.Reason
}

// A helper may forward exhaustive child alternatives, but an unresolved
// select in the helper itself must not disappear merely because it also
// launches a child. Those synchronous continuation effects remain unknown.
func (summary Summary) workerAlternativesOnly() bool {
	if !summary.AlternativesComplete || !summary.hasWorkerAlternatives() {
		return false
	}
	for _, choice := range summary.Choices {
		if choice.Worker == nil {
			return false
		}
	}
	return true
}

func (engine *Engine) bindWorker(
	worker WorkerSummary, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction,
) (WorkerSummary, string) {
	bound := WorkerSummary{Spawn: worker.Spawn, Site: instruction.Pos(), Prefix: worker.Prefix, Branches: worker.Branches}
	var reason string
	bound.Operations, reason = engine.bindOperations(worker.Operations, bindings, instruction)
	if reason != "" {
		return WorkerSummary{}, reason
	}
	for _, path := range worker.Alternatives {
		operations, reason := engine.bindOperations(path, bindings, instruction)
		if reason != "" {
			return WorkerSummary{}, reason
		}
		bound.Alternatives = append(bound.Alternatives, operations)
	}
	return bound, ""
}

func (engine *Engine) bindOperations(operations []Operation, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction) ([]Operation, string) {
	bound := make([]Operation, 0, len(operations))
	for _, op := range operations {
		if !engine.budget.Spend() {
			return nil, "protocol-budget-exhausted"
		}
		resource, ok := engine.bind(op.Resource, bindings, instruction)
		if !ok {
			return nil, "protocol-channel-binding-unknown"
		}
		op.Resource, op.Site = resource, instruction.Pos()
		bound = append(bound, op)
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
