package concurrencyfacts

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Exact repetition is not a loop fixed point. Only a zero-based, unit-step
// counter with a literal bound and one straight-line body is expanded. The
// counter cannot escape into a worker or affect resource identity. Dynamic
// counts, breaks, nested loops, and iteration-local objects remain unknown.
const maxProtocolIterations = 4

func (engine *Engine) collectCountedLoops(function *ssa.Function, root bool) Summary {
	if !detachedRecovery(function) {
		engine.recordBlockCutoff(function.Recover, cutoffRecovery)
		return Summary{Reason: ReasonControlFlowUnknown}
	}
	var result Summary
	seen := make(map[*ssa.BasicBlock]bool)
	block := function.Blocks[0]
	for block != nil {
		if seen[block] || !engine.budget.Spend() {
			engine.recordBlockCutoff(block, cutoffLoop)
			return Summary{Reason: ReasonControlFlowUnknown}
		}
		seen[block] = true
		if len(block.Succs) == 2 {
			// Failure is attributed to the header, not an imagined iteration:
			// the count proof does not establish that its body can finish.
			loop := ssaflow.ProveCountedLoop(block, maxProtocolIterations, engine.budget)
			if !loop.Proven() || loop.CounterUsed || seen[loop.Body] || !engine.repeatableBody(loop.Body) {
				engine.recordBlockCutoff(block, cutoffLoop)
				return Summary{Reason: ReasonControlFlowUnknown}
			}
			seen[loop.Body] = true
			for range loop.Count {
				if reason := engine.collectBlock(&result, loop.Body, root); reason != ReasonNone {
					return Summary{Reason: reason}
				}
			}
			block = loop.Exit
			continue
		}
		if reason := engine.collectBlock(&result, block, root); reason != ReasonNone {
			return Summary{Reason: reason}
		}
		if len(block.Succs) == 0 {
			block = nil
		} else {
			block = block.Succs[0]
		}
	}
	if len(result.deferred) != 0 || result.hasWorkerAlternatives() {
		return Summary{Reason: ReasonControlFlowUnknown}
	}
	return result
}

// Replaying an allocation site would merge distinct runtime resources. Deferred
// calls accumulate across iterations, rather than settling each iteration.
func (engine *Engine) repeatableBody(body *ssa.BasicBlock) bool {
	for _, instruction := range body.Instrs {
		if !engine.budget.Spend() {
			return false
		}
		switch instruction.(type) {
		case *ssa.Alloc, *ssa.MakeChan, *ssa.Defer:
			return false
		}
	}
	return true
}

// A loop that synchronizes nothing and provably ends is invisible to the
// ordered effects: however many times it runs, it adds no operation, and the
// code after it is reached. The acyclic collectors therefore see such a loop
// as one node whose successors are its exits. The proof has two parts.
// ssaflow.BoundedLoop shows that every loop in it counts up to a bound fixed
// before the loop, so it ends. And every instruction in it collects to no
// effect of any kind: no operation, worker, hole, defer, select, cancellation
// input, or result condition, so nothing in it can block, wait, or leave
// work behind. A loop driven by anything but such a counter, including a map
// range, a Boolean flag, or a receive, is not proven to end and stays a
// cycle, which the collectors decline.

// acyclicFlow is a function's blocks in topological order with its foldable
// loops folded into their headers.
type acyclicFlow struct {
	order  []*ssa.BasicBlock
	folded map[*ssa.BasicBlock]foldedLoop
}

// foldedLoop is a loop the collectors treat as one node. replay lists the
// blocks whose effects they add once, in order: none for a quiet loop, one
// iteration for a worker pool (see replicated_workers.go).
type foldedLoop struct {
	loop   ssaflow.NaturalLoop
	replay []*ssa.BasicBlock
}

// isFolded reports whether block is the header of a folded loop, whose own
// instructions the collectors replace with its replay.
func (flow acyclicFlow) isFolded(block *ssa.BasicBlock) bool {
	_, folded := flow.folded[block]
	return folded
}

// successors returns a block's successors, or a folded loop's exits.
func (flow acyclicFlow) successors(block *ssa.BasicBlock) []*ssa.BasicBlock {
	if folded, ok := flow.folded[block]; ok {
		return folded.loop.Exits
	}
	return block.Succs
}

func (engine *Engine) foldLoops(function *ssa.Function, root bool) map[*ssa.BasicBlock]foldedLoop {
	folded := make(map[*ssa.BasicBlock]foldedLoop)
	loops, ok := ssaflow.OutermostLoops(function, engine.budget)
	if !ok {
		return folded
	}
	for _, loop := range loops {
		if !ssaflow.BoundedLoop(loop, engine.budget) {
			continue
		}
		if engine.quietLoop(loop, root) {
			folded[loop.Header] = foldedLoop{loop: loop}
			continue
		}
		// An exact small count is unrolled later, which is more precise than
		// one representative iteration.
		if ssaflow.ProveCountedLoop(loop.Header, maxProtocolIterations, engine.budget).Proven() {
			continue
		}
		if replay, ok := engine.workerPool(loop, root); ok {
			folded[loop.Header] = foldedLoop{loop: loop, replay: replay}
		}
	}
	return folded
}

func (engine *Engine) quietLoop(loop ssaflow.NaturalLoop, root bool) bool {
	var effects Summary
	for _, block := range loop.Blocks {
		if engine.collectBlock(&effects, block, root) != ReasonNone {
			return false
		}
	}
	return noEffects(effects)
}

// replayLoop adds a folded loop's replayed iteration to state.
func (engine *Engine) replayLoop(state *Summary, folded foldedLoop, root bool) Reason {
	before := len(state.Workers)
	for _, block := range folded.replay {
		if reason := engine.collectBlock(state, block, root); reason != ReasonNone {
			return reason
		}
	}
	markReplicated(state, before)
	return ReasonNone
}
