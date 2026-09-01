package goroutineownership

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Counted-join evidence pairs bounded goroutine launches with the corresponding
// receives or helper join. Bounds must resolve to the same SSA value or a
// finite aggregate; dynamic and ambiguous counts deliberately remain unproven.

func matchingCountedJoin(function *ssa.Function, spawn *ssa.Go, signals []ssa.Value) bool {
	spawnBound := loopBound(function, spawn.Block())
	if spawnBound == nil {
		spawnBound = stableMapRangeBound(function, spawn)
	}
	if spawnBound == nil {
		return finiteAggregateJoin(function, spawn, signals)
	}
	for _, block := range function.Blocks {
		joinBound := loopBound(function, block)
		if joinBound == nil || !ssaflow.SameValue(spawnBound, joinBound) {
			continue
		}
		for _, instruction := range block.Instrs {
			receive, ok := instruction.(*ssa.UnOp)
			if ok && receive.Op == token.ARROW && ssaflow.SameAsAny(receive.X, signals) && receive.Pos() > spawn.Pos() {
				return true
			}
		}
	}
	return matchingCountedHelperJoin(function, spawn, signals, spawnBound) || finiteAggregateJoin(function, spawn, signals)
}

func stableMapRangeBound(function *ssa.Function, spawn *ssa.Go) ssa.Value { //nolint:ireturn // The map remains an SSA value for bound comparison.
	header := enclosingLoopHeader(function, spawn.Block())
	if header == nil {
		return nil
	}
	for _, instruction := range header.Instrs {
		next, ok := instruction.(*ssa.Next)
		if !ok {
			continue
		}
		ranged, ok := next.Iter.(*ssa.Range)
		if !ok {
			continue
		}
		if _, mapType := ranged.X.Type().Underlying().(*types.Map); !mapType || mapMayChangeAfterSpawn(function, spawn, ranged.X) {
			return nil
		}
		// A race-free map range launches exactly len(map) workers when the map is
		// not changed before the join. Loom uses this shape to receive one terminal
		// signal for every agent worker.
		// https://github.com/teradata-labs/loom/blob/9d8c8c672ce22e1d51374bdfb2064fa0d8719968/pkg/server/multi_agent_shared_memory_test.go#L239-L265
		return ranged.X
	}
	return nil
}

func mapMayChangeAfterSpawn(function *ssa.Function, spawn *ssa.Go, target ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if instruction == spawn || !ssaflow.InstructionMayFollow(spawn, instruction) {
				continue
			}
			if update, ok := instruction.(*ssa.MapUpdate); ok && ssaflow.SameValue(update.Map, target) {
				return true
			}
			common := ssaflow.InstructionCall(instruction)
			if common == nil {
				continue
			}
			if ssaflow.CallMatchesSymbol(common, syntax.Builtin("len")) {
				continue
			}
			for _, argument := range common.Args {
				if ssaflow.SameValue(argument, target) {
					return true
				}
			}
		}
	}
	return false
}

func matchingCountedHelperJoin(function *ssa.Function, spawn *ssa.Go, signals []ssa.Value, spawnBound ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if instruction.Pos() > spawn.Pos() && callMatchesCountedHelper(instruction, signals, spawnBound) {
				return true
			}
		}
	}
	return false
}

func callMatchesCountedHelper(instruction ssa.Instruction, signals []ssa.Value, spawnBound ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	for signalIndex, argument := range common.Args {
		if !ssaflow.SameAsAny(argument, signals) || signalIndex >= len(callee.Params) {
			continue
		}
		for boundIndex, boundArgument := range common.Args {
			if boundIndex < len(callee.Params) &&
				sameBound(boundArgument, spawnBound) &&
				helperReceivesCount(callee, signalIndex, boundIndex) {
				return true
			}
		}
	}
	return false
}

func helperReceivesCount(function *ssa.Function, signalIndex, boundIndex int) bool {
	signal, bound := function.Params[signalIndex], function.Params[boundIndex]
	if eventuallyReceivesCount(function, signal, bound) {
		return true
	}
	for _, block := range function.Blocks {
		if !ssaflow.SameValue(loopBound(function, block), bound) {
			continue
		}
		for _, instruction := range block.Instrs {
			receive, ok := instruction.(*ssa.UnOp)
			if ok && receive.Op == token.ARROW && ssaflow.SameValue(receive.X, signal) {
				return true
			}
		}
	}
	return false
}

func eventuallyReceivesCount(function *ssa.Function, signal, count ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if common == nil || !strings.Contains(strings.ToLower(ssaflow.CallName(common)), "eventually") {
				continue
			}
			for _, argument := range common.Args {
				if callbackReceives(argument, signal) && ssaflow.ValueContainsValue(argument, count) {
					return true
				}
			}
		}
	}
	return false
}

func callbackReceives(value, signal ssa.Value) bool {
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentMakeInterface,
	); ok {
		return callbackReceives(inner, signal)
	}
	switch typed := value.(type) {
	case *ssa.MakeClosure:
		return valueReceivesAny(typed, []ssa.Value{signal}, map[ssa.Value]bool{})
	}
	return false
}

func sameBound(first, second ssa.Value) bool {
	if ssaflow.SameValue(first, second) {
		return true
	}
	left, leftOK := first.(*ssa.Const)
	right, rightOK := second.(*ssa.Const)
	return leftOK && rightOK && left.Value != nil && right.Value != nil && left.Value.ExactString() == right.Value.ExactString()
}

func finiteAggregateJoin(function *ssa.Function, spawn *ssa.Go, signals []ssa.Value) bool {
	// A map populated with each worker's signal is a finite join set.
	// Accept it only when a later receive derives from that exact aggregate;
	// merely storing signals does not establish that the caller waits for them.
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			update, ok := instruction.(*ssa.MapUpdate)
			if !ok || !ssaflow.SameAsAny(update.Value, signals) {
				continue
			}
			for _, receiveBlock := range function.Blocks {
				for _, candidate := range receiveBlock.Instrs {
					if candidate.Pos() <= spawn.Pos() || !receiveFromAggregate(candidate, update.Map) {
						continue
					}
					return true
				}
			}
		}
	}
	return false
}

func receiveFromAggregate(instruction ssa.Instruction, aggregate ssa.Value) bool {
	if receive, ok := instruction.(*ssa.UnOp); ok && receive.Op == token.ARROW {
		return ssaflow.ValueDerivesFrom(receive.X, aggregate, map[ssa.Value]bool{})
	}
	if selection, ok := instruction.(*ssa.Select); ok {
		for _, state := range selection.States {
			if state.Dir == types.RecvOnly && ssaflow.ValueDerivesFrom(state.Chan, aggregate, map[ssa.Value]bool{}) {
				return true
			}
		}
	}
	return false
}

func loopBound(function *ssa.Function, body *ssa.BasicBlock) ssa.Value { //nolint:ireturn // Bounds retain their SSA form for alias comparison.
	selected := enclosingLoopHeader(function, body)
	if selected == nil {
		return nil
	}
	candidates := []*ssa.BasicBlock{selected}
	for _, predecessor := range selected.Preds {
		if selected.Dominates(predecessor) {
			candidates = append(candidates, predecessor)
		}
	}
	for _, candidate := range candidates {
		if bound := loopComparisonBound(candidate, selected); bound != nil {
			if call, ok := bound.(*ssa.Call); ok && ssaflow.CallMatchesSymbol(call.Common(), syntax.Builtin("len")) && len(call.Common().Args) == 1 {
				return call.Common().Args[0]
			}
			return bound
		}
	}
	return nil
}

func enclosingLoopHeader(function *ssa.Function, body *ssa.BasicBlock) *ssa.BasicBlock {
	var selected *ssa.BasicBlock
	for _, header := range function.Blocks {
		if !header.Dominates(body) || !loopHeader(header) {
			continue
		}
		if selected == nil || selected.Dominates(header) {
			selected = header
		}
	}
	return selected
}

func loopComparisonBound(block, header *ssa.BasicBlock) ssa.Value { //nolint:ireturn // Bounds retain their SSA form for alias comparison.
	if len(block.Instrs) == 0 {
		return nil
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return nil
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok {
		return nil
	}
	if induction := loopInduction(comparison.X, header); induction != nil {
		if constantZero(comparison.Y) {
			if initial := loopInitialValue(induction, header); initial != nil {
				return initial
			}
		}
		return comparison.Y
	}
	if induction := loopInduction(comparison.Y, header); induction != nil {
		if constantZero(comparison.X) {
			if initial := loopInitialValue(induction, header); initial != nil {
				return initial
			}
		}
		return comparison.X
	}
	return nil
}

func loopInitialValue(induction *ssa.Phi, header *ssa.BasicBlock) ssa.Value { //nolint:ireturn // The initial loop value retains its SSA form.
	for index, predecessor := range header.Preds {
		if index < len(induction.Edges) && !header.Dominates(predecessor) {
			return induction.Edges[index]
		}
	}
	return nil
}

func constantZero(value ssa.Value) bool {
	literal, ok := value.(*ssa.Const)
	return ok && literal.Value != nil && constant.Sign(literal.Value) == 0
}

func loopInduction(value ssa.Value, header *ssa.BasicBlock) *ssa.Phi {
	if induction, ok := value.(*ssa.Phi); ok && induction.Block() == header {
		return induction
	}
	step, ok := value.(*ssa.BinOp)
	if !ok || step.Op != token.ADD {
		return nil
	}
	if induction, ok := step.X.(*ssa.Phi); ok && induction.Block() == header {
		return induction
	}
	if induction, ok := step.Y.(*ssa.Phi); ok && induction.Block() == header {
		return induction
	}
	return nil
}

func loopHeader(header *ssa.BasicBlock) bool {
	return slices.ContainsFunc(header.Preds, header.Dominates)
}
