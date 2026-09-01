package goroutineownership

import (
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	ssautil "github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/ssa"
)

func matchingCountedJoin(function *ssa.Function, spawn *ssa.Go, signals []ssa.Value) bool {
	spawnBound := loopBound(function, spawn.Block())
	if spawnBound == nil {
		return finiteAggregateJoin(function, spawn, signals)
	}
	for _, block := range function.Blocks {
		joinBound := loopBound(function, block)
		if joinBound == nil || !ssautil.AliasesValue(spawnBound, joinBound) {
			continue
		}
		for _, instruction := range block.Instrs {
			receive, ok := instruction.(*ssa.UnOp)
			if ok && receive.Op == token.ARROW && ssautil.AliasesAny(receive.X, signals) && receive.Pos() > spawn.Pos() {
				return true
			}
		}
	}
	return matchingCountedHelperJoin(function, spawn, signals, spawnBound) || finiteAggregateJoin(function, spawn, signals)
}

func matchingCountedHelperJoin(function *ssa.Function, spawn *ssa.Go, signals []ssa.Value, spawnBound ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssautil.InstructionCall(instruction)
			if common == nil || instruction.Pos() <= spawn.Pos() {
				continue
			}
			callee := common.StaticCallee()
			if callee == nil {
				continue
			}
			for signalIndex, argument := range common.Args {
				if !ssautil.AliasesAny(argument, signals) || signalIndex >= len(callee.Params) {
					continue
				}
				for boundIndex, boundArgument := range common.Args {
					if boundIndex >= len(callee.Params) || !sameBound(boundArgument, spawnBound) {
						continue
					}
					if eventuallyReceivesCount(callee, callee.Params[signalIndex], callee.Params[boundIndex]) {
						return true
					}
					for _, helperBlock := range callee.Blocks {
						if !ssautil.AliasesValue(loopBound(callee, helperBlock), callee.Params[boundIndex]) {
							continue
						}
						for _, candidate := range helperBlock.Instrs {
							receive, ok := candidate.(*ssa.UnOp)
							if ok && receive.Op == token.ARROW && ssautil.AliasesValue(receive.X, callee.Params[signalIndex]) {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func eventuallyReceivesCount(function *ssa.Function, signal, count ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssautil.InstructionCall(instruction)
			if common == nil || !strings.Contains(strings.ToLower(ssautil.CallName(common)), "eventually") {
				continue
			}
			for _, argument := range common.Args {
				if callbackReceives(argument, signal) && ssautil.ValueOwnsValue(argument, count) {
					return true
				}
			}
		}
	}
	return false
}

func callbackReceives(value, signal ssa.Value) bool {
	switch typed := value.(type) {
	case *ssa.MakeInterface:
		return callbackReceives(typed.X, signal)
	case *ssa.ChangeInterface:
		return callbackReceives(typed.X, signal)
	case *ssa.MakeClosure:
		return valueReceivesAny(typed, []ssa.Value{signal}, map[ssa.Value]bool{})
	}
	return false
}

func sameBound(first, second ssa.Value) bool {
	if ssautil.AliasesValue(first, second) {
		return true
	}
	left, leftOK := first.(*ssa.Const)
	right, rightOK := second.(*ssa.Const)
	return leftOK && rightOK && left.Value != nil && right.Value != nil && left.Value.ExactString() == right.Value.ExactString()
}

func finiteAggregateJoin(function *ssa.Function, spawn *ssa.Go, signals []ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			update, ok := instruction.(*ssa.MapUpdate)
			if !ok || !ssautil.AliasesAny(update.Value, signals) {
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
		return ssautil.ValueDerivesFrom(receive.X, aggregate, map[ssa.Value]bool{})
	}
	if selection, ok := instruction.(*ssa.Select); ok {
		for _, state := range selection.States {
			if state.Dir == types.RecvOnly && ssautil.ValueDerivesFrom(state.Chan, aggregate, map[ssa.Value]bool{}) {
				return true
			}
		}
	}
	return false
}

func loopBound(function *ssa.Function, body *ssa.BasicBlock) ssa.Value { //nolint:ireturn // Bounds retain their SSA form for alias comparison.
	var selected *ssa.BasicBlock
	for _, header := range function.Blocks {
		if !header.Dominates(body) || !loopHeader(header) {
			continue
		}
		if selected == nil || selected.Dominates(header) {
			selected = header
		}
	}
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
			if call, ok := bound.(*ssa.Call); ok && ssautil.CallName(call.Common()) == "len" && len(call.Common().Args) == 1 {
				return call.Common().Args[0]
			}
			return bound
		}
	}
	return nil
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
	for _, predecessor := range header.Preds {
		if header.Dominates(predecessor) {
			return true
		}
	}
	return false
}
