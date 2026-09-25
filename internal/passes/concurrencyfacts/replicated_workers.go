package concurrencyfacts

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

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
		if dominatesLatches(loop, block) {
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

// dominatesLatches reports whether block runs on every iteration.
func dominatesLatches(loop ssaflow.NaturalLoop, block *ssa.BasicBlock) bool {
	for _, predecessor := range loop.Header.Preds {
		if loop.Contains(predecessor) && !block.Dominates(predecessor) {
			return false
		}
	}
	return true
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
