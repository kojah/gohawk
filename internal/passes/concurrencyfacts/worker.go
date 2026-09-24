package concurrencyfacts

import "golang.org/x/tools/go/ssa"

// Root queries own child launch identity and the bounded alternative sequences.
// Function summaries never silently turn a nested launch into synchronous
// effects; opaque participants invalidate the protocol proof instead.

// maxWorkers bounds the number of separately ordered child sequences in a
// root query. Launches in loops remain unknown regardless of this limit.
const maxWorkers = 4

func (engine *Engine) appendGo(result *Summary, instruction *ssa.Go, root bool) string {
	// A root records each statically known launch separately. A fifth
	// participant or a child with unavailable effects makes the *whole*
	// protocol unknown: otherwise a missing alternate signal or unlock
	// could turn a feasible wait into a false deadlock proof.
	if !root || len(result.Workers) >= maxWorkers {
		return "protocol-participants-unknown"
	}
	called := engine.instantiate(instruction)
	if !called.Complete() {
		if called.Reason != "protocol-select-alternatives" || len(called.Choices) != 1 || !completeChoice(called.Choices[0]) {
			return called.Reason
		}
		worker := WorkerSummary{Spawn: instruction, Prefix: len(result.Operations)}
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
		Operations: called.Operations, Spawn: instruction, Prefix: len(result.Operations),
	})
	return ""
}

func (summary Summary) hasWorkerAlternatives() bool {
	for _, worker := range summary.Workers {
		if len(worker.Alternatives) != 0 {
			return true
		}
	}
	return false
}
