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
	if !trivialRecovery(function) {
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
