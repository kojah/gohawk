package lifecycle

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// ElementLoopReleasesEach reports whether the element a range loop reads at
// address is used only as the receiver of a call to one of methods that runs
// on every iteration: its block dominates every back edge to the header. Such
// a loop releases each element it reads, and together its iterations read
// them all, so leaving it has released the whole slice. Both a caller's local
// collection and a helper's parameter summary ask this one question.
func ElementLoopReleasesEach(loop ssaflow.ElementLoop, address *ssa.IndexAddr, methods []string) bool {
	released := false
	for _, user := range *address.Referrers() {
		if _, ok := user.(*ssa.DebugRef); ok {
			continue
		}
		load, ok := user.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			return false
		}
		for _, use := range *load.Referrers() {
			if _, ok := use.(*ssa.DebugRef); ok {
				continue
			}
			call, ok := use.(*ssa.Call)
			if !ok || ssaflow.CallReceiver(call.Common()) != load || !slices.Contains(methods, ssaflow.CallName(call.Common())) ||
				!runsEveryIteration(loop, call.Block()) {
				return false
			}
			released = true
		}
	}
	return released
}

func runsEveryIteration(loop ssaflow.ElementLoop, block *ssa.BasicBlock) bool {
	for _, predecessor := range loop.Loop.Header.Preds {
		if loop.Loop.Contains(predecessor) && !block.Dominates(predecessor) {
			return false
		}
	}
	return true
}
