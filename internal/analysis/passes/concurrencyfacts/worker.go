package concurrencyfacts

import (
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Function summaries retain child templates; root queries instantiate each
// call as a distinct child. A nested launch in a worker body remains unknown.

// maxWorkers bounds the number of separately ordered child sequences in a
// root query, including launches expanded from an exact counted loop.
const maxWorkers = 4

func (engine *Engine) appendGo(result *Summary, instruction *ssa.Go) Reason {
	// A function records each statically known launch separately. A fifth
	// participant or a child with unavailable effects makes the *whole*
	// protocol unknown: otherwise a missing alternate signal or unlock
	// could turn a feasible wait into a false deadlock proof.
	if len(result.Workers) >= maxWorkers {
		return ReasonParticipantsUnknown
	}
	called := engine.instantiate(instruction)
	result.CancellationInputs = append(result.CancellationInputs, called.CancellationInputs...)
	if len(called.Paths) != 0 {
		worker := WorkerSummary{Spawn: instruction, Site: instruction.Pos(), Prefix: len(result.Operations), Branches: true}
		for _, path := range called.Paths {
			if !composableLinear(path) || len(path.Workers) != 0 || len(path.Paths) != 0 {
				return ReasonWorkerEffectsUnknown
			}
			requireCancellation(result, path.CancellationInputs)
			worker.Alternatives = append(worker.Alternatives, path.Operations)
			worker.AlternativeConditions = append(worker.AlternativeConditions, path.Conditions)
		}
		result.Workers = append(result.Workers, worker)
		return ReasonNone
	}
	if len(called.Workers) != 0 {
		return ReasonWorkerEffectsUnknown
	}
	if !composableLinear(called) {
		if called.Reason != ReasonSelectAlternatives || len(called.Choices) != 1 || !completeChoice(called.Choices[0]) {
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
		return ReasonNone
	}
	result.Workers = append(result.Workers, WorkerSummary{
		Operations: called.Operations, Spawn: instruction, Site: instruction.Pos(), Prefix: len(result.Operations),
	})
	return ReasonNone
}

func (engine *Engine) appendCall(result *Summary, instruction *ssa.Call) Reason {
	called := engine.callSummary(instruction)
	return appendCalled(result, called, instruction)
}

func appendCalled(result *Summary, called Summary, instruction ssa.CallInstruction) Reason {
	result.CancellationInputs = append(result.CancellationInputs, called.CancellationInputs...)
	result.Conditions = append(result.Conditions, called.Conditions...)
	if len(called.Workers) != 0 {
		if !composableLinear(called) && !called.workerAlternativesOnly() || len(result.Workers)+len(called.Workers) > maxWorkers {
			return ReasonParticipantsUnknown
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
		return ReasonNone
	}
	if called.Reason == ReasonContextBindingRequired || called.Reason == ReasonCallbackBindingRequired ||
		called.Reason == ReasonReplicatedWorkers {
		return ReasonNone
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
	worker WorkerSummary, bindings []ssacall.CallBinding, instruction ssa.CallInstruction,
) (WorkerSummary, Reason) {
	bound := WorkerSummary{
		Spawn: worker.Spawn, Site: instruction.Pos(), Prefix: worker.Prefix, Branches: worker.Branches, Replicated: worker.Replicated,
	}
	var reason Reason
	bound.Operations, reason = engine.bindOperations(worker.Operations, bindings, instruction)
	if reason != ReasonNone {
		return WorkerSummary{}, reason
	}
	for index, path := range worker.Alternatives {
		operations, reason := engine.bindOperations(path, bindings, instruction)
		if reason != ReasonNone {
			return WorkerSummary{}, reason
		}
		bound.Alternatives = append(bound.Alternatives, operations)
		if index < len(worker.AlternativeConditions) {
			bound.AlternativeConditions = append(bound.AlternativeConditions,
				boundConditions(worker.AlternativeConditions[index], bindings, instruction.Pos()))
		}
	}
	return bound, ReasonNone
}

func (engine *Engine) bindOperations(operations []Operation, bindings []ssacall.CallBinding, instruction ssa.CallInstruction) ([]Operation, Reason) {
	bound := make([]Operation, 0, len(operations))
	for _, op := range operations {
		if !engine.budget.Spend() {
			return nil, ReasonBudgetExhausted
		}
		if op.Kind == Invoke {
			// Holes are filled only in the linear sequence; see callbacks.go.
			return nil, ReasonCallbackUnknown
		}
		resource, ok := engine.bind(op.Resource, bindings, instruction)
		if !ok {
			return nil, ReasonChannelBindingUnknown
		}
		op.Resource, op.Site = resource, instruction.Pos()
		bound = append(bound, op)
	}
	return bound, ReasonNone
}

func (summary Summary) hasWorkerAlternatives() bool {
	for _, worker := range summary.Workers {
		if len(worker.Alternatives) != 0 {
			return true
		}
	}
	return false
}

// A worker pool launches identical goroutines from a bounded loop:
//
//	for range n {
//		wg.Add(1)
//		go func() { defer wg.Done(); work() }()
//	}
//
// The number of copies is unknown, so the summary replays one iteration and
// marks the workers it launches Replicated: each stands for one or more
// identical copies. That reading is sound only for a property that more
// identical copies cannot break, such as a copy stuck on a lock its parent
// holds, which no other copy stuck on the same lock can release. Copies
// can partner each other on a channel, though, and change any count of sends
// and receives. So a replicated worker makes a summary incomplete, and only
// a consumer that proves such a property opts in through Representatives.
//
// The loop must be proven bounded, so the code after it runs. The blocks that
// run on every iteration may only add to a WaitGroup and launch goroutines;
// the blocks that run on some iterations must be quiet. Every resource the
// iteration or its workers touch must be the same object on every iteration:
// defined before the loop, or a path whose root is. A channel or group made
// inside the loop is a new object each time and leaves the loop unknown.

// hasReplicatedWorkers reports whether any worker stands for several copies.
func (summary Summary) hasReplicatedWorkers() bool {
	for _, worker := range summary.Workers {
		if worker.Replicated {
			return true
		}
	}
	return false
}

// finishReplicas marks a summary that still has replicated workers.
func finishReplicas(summary Summary) Summary {
	switch {
	case summary.Reason == ReasonNone && summary.hasReplicatedWorkers():
		summary.Reason = ReasonReplicatedWorkers
	case summary.Reason == ReasonReplicatedWorkers && !summary.hasReplicatedWorkers():
		summary.Reason = ReasonNone
	}
	return summary
}

// Representatives reads every replicated worker as one worker. Use it only
// for a property that holds for any number of identical copies once it holds
// for one; see the file comment.
func (summary Summary) Representatives() Summary {
	summary = cloneEffects(summary)
	for index := range summary.Workers {
		summary.Workers[index].Replicated = false
	}
	for index := range summary.Paths {
		summary.Paths[index] = summary.Paths[index].Representatives()
	}
	return finishReplicas(summary)
}

// workerPool returns the loop's every-iteration blocks when the loop is a
// worker pool, for the collectors to replay once.
func (engine *Engine) workerPool(loop ssaflow.NaturalLoop, root bool) ([]*ssa.BasicBlock, bool) {
	var every []*ssa.BasicBlock
	var optional Summary
	for _, block := range loop.Blocks {
		if loop.DominatesBackEdges(block) {
			every = append(every, block)
			continue
		}
		if engine.collectBlock(&optional, block, root) != ReasonNone || !noEffects(optional) {
			return nil, false
		}
	}
	var iteration Summary
	for _, block := range every {
		if engine.collectBlock(&iteration, block, root) != ReasonNone {
			return nil, false
		}
	}
	if len(iteration.Workers) == 0 || len(iteration.Choices) != 0 || len(iteration.deferred) != 0 ||
		len(iteration.CancellationInputs) != 0 || len(iteration.Conditions) != 0 {
		return nil, false
	}
	for _, operation := range iteration.Operations {
		if operation.Kind != GroupAdd || !engine.invariantResource(loop, operation.Resource) {
			return nil, false
		}
	}
	for _, worker := range iteration.Workers {
		if len(worker.Alternatives) != 0 {
			return nil, false
		}
		for _, operation := range worker.Operations {
			if !engine.invariantResource(loop, operation.Resource) {
				return nil, false
			}
		}
	}
	return every, true
}

// markReplicated marks the workers launched since before as replicated.
func markReplicated(summary *Summary, before int) {
	for index := before; index < len(summary.Workers); index++ {
		summary.Workers[index].Replicated = true
	}
}

// invariantResource reports whether reference names the same object on every
// iteration: its value, or the root of its path, is defined outside the loop.
func (engine *Engine) invariantResource(loop ssaflow.NaturalLoop, reference Reference) bool {
	value := reference.Value
	if reference.Projection.Depth > 0 {
		value = reference.Projection.Root
	}
	if outsideLoop(loop, value) {
		return true
	}
	path, ok := engine.identityPath(value)
	return ok && path.Depth > 0 && outsideLoop(loop, path.Root)
}

func outsideLoop(loop ssaflow.NaturalLoop, value ssa.Value) bool {
	instruction, ok := value.(ssa.Instruction)
	return !ok || !loop.Contains(instruction.Block())
}

func noEffects(summary Summary) bool {
	return len(summary.Operations) == 0 && len(summary.Workers) == 0 && len(summary.Choices) == 0 &&
		len(summary.deferred) == 0 && len(summary.CancellationInputs) == 0 && len(summary.Conditions) == 0
}
