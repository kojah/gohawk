package goroutineownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Guarded local joins cover an optional worker whose stop channel and wait
// group are created together. The launch itself proves that the exact local
// channel is non-nil, so a later nil guard cannot expose an unjoined return on
// that path. The proof stops at one stable closure binding, an exact close,
// and an exact group wait; it does not infer general relationships between
// branches or lifecycle values.

func (analysis goroutineAnalysis) guardedLocalStopJoin() ssa.Instruction {
	if len(analysis.groups) == 0 {
		return nil
	}
	for _, stop := range stableLocalStopChannels(analysis.function, analysis.spawn) {
		guard := nonNilStopGuard(analysis.spawn, stop)
		closed := exactStopCloseAfter(analysis.spawn, stop)
		wait := exactGroupWaitAfter(analysis.spawn, analysis.groups)
		if guard == nil || closed == nil || wait == nil || !ssaflow.InstructionDominates(closed, wait) ||
			!nonNilSuccessorDominates(guard, wait) || ssaflow.UnownedReturnAssumingNonNil(
			analysis.spawn,
			stop,
			analysis.instructionOwnsGoroutine,
			analysis.returnOwnsGoroutine,
		) {
			continue
		}
		// Rainier creates its resize worker only after assigning the local stop
		// channel, then closes that exact channel and waits for the exact group
		// beneath a nil guard:
		// https://github.com/tokencanopy/rainier/blob/855b2e7c276a60a2f65f141d1071cf03be38d6e3/internal/attachio/attachio.go#L267-L287
		// https://github.com/tokencanopy/rainier/blob/855b2e7c276a60a2f65f141d1071cf03be38d6e3/internal/attachio/attachio.go#L437-L443
		return wait
	}
	return nil
}

func stableLocalStopChannels(parent *ssa.Function, spawn *ssa.Go) []ssa.Value {
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return nil
	}
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return nil
	}
	var stops []ssa.Value
	for index, free := range function.FreeVars {
		if index >= len(closure.Bindings) || !functionReceivesParameter(function, free, map[*ssa.Function]bool{}) {
			continue
		}
		binding := closure.Bindings[index]
		stop := singleDominatingChannelStore(parent, spawn, closure, binding)
		if stop != nil {
			stops = append(stops, stop)
		}
	}
	return stops
}

func singleDominatingChannelStore(
	parent *ssa.Function,
	spawn *ssa.Go,
	closure *ssa.MakeClosure,
	binding ssa.Value,
) ssa.Value { //nolint:ireturn // Preserve the concrete channel value.
	if binding == nil || binding.Referrers() == nil {
		return nil
	}
	var stored ssa.Value
	for _, reference := range *binding.Referrers() {
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.UnOp:
			if typed.Op == token.MUL && typed.X == binding {
				continue
			}
		case *ssa.MakeClosure:
			if typed == closure {
				continue
			}
		case *ssa.Store:
			created, ok := typed.Val.(*ssa.MakeChan)
			if ok && typed.Addr == binding && created.Parent() == parent && stored == nil &&
				ssaflow.InstructionDominates(typed, spawn) && !ssaflow.BlockInCycle(typed.Block()) &&
				!ssaflow.BlockInCycle(spawn.Block()) {
				stored = created
				continue
			}
		}
		// An opaque use can retain or mutate the captured address. A single SSA
		// store can also execute repeatedly in a loop. Neither shape preserves
		// the one channel instance required by this proof.
		return nil
	}
	if stored == nil {
		return nil
	}
	return stored
}

func nonNilStopGuard(spawn *ssa.Go, stop ssa.Value) *ssa.If {
	for _, block := range spawn.Parent().Blocks {
		for _, instruction := range block.Instrs {
			branch, ok := instruction.(*ssa.If)
			if !ok || !ssaflow.InstructionMayFollow(spawn, branch) {
				continue
			}
			comparison, ok := branch.Cond.(*ssa.BinOp)
			if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
				continue
			}
			if ssaflow.SameValue(comparison.X, stop) && ssaflow.DefinitelyNil(comparison.Y) ||
				ssaflow.SameValue(comparison.Y, stop) && ssaflow.DefinitelyNil(comparison.X) {
				return branch
			}
		}
	}
	return nil
}

func exactStopCloseAfter(spawn *ssa.Go, stop ssa.Value) ssa.Instruction {
	for _, block := range spawn.Parent().Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if common != nil && ssaflow.InstructionMayFollow(spawn, instruction) &&
				ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1 &&
				ssaflow.SameValue(common.Args[0], stop) {
				return instruction
			}
		}
	}
	return nil
}

func exactGroupWaitAfter(spawn *ssa.Go, groups []ssa.Value) ssa.Instruction {
	for _, block := range spawn.Parent().Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if common != nil && ssaflow.InstructionMayFollow(spawn, instruction) &&
				ssaflow.CallMatchesSymbol(common, waitGroupWait) &&
				ssaflow.SameAsAny(ssaflow.CallReceiver(common), groups) {
				return instruction
			}
		}
	}
	return nil
}

func nonNilSuccessorDominates(guard *ssa.If, after ssa.Instruction) bool {
	comparison, ok := guard.Cond.(*ssa.BinOp)
	if !ok || len(guard.Block().Succs) != 2 {
		return false
	}
	nonNil := guard.Block().Succs[0]
	if comparison.Op == token.EQL {
		nonNil = guard.Block().Succs[1]
	}
	return nonNil.Dominates(after.Block())
}
