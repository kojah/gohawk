package concurrencyfacts

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

const maxSelectArms = 8

// A select executes exactly one communication, or its default arm. We keep
// every arm explicit and incomplete for linear consumers; flattening them
// would invent an unavoidable wait.
func (engine *Engine) appendSelect(result *Summary, selection *ssa.Select) string {
	if len(selection.States) == 0 || len(selection.States) > maxSelectArms {
		return "protocol-select-alternatives-unknown"
	}
	choice := SelectChoice{Prefix: len(result.Operations), Site: selection.Pos()}
	for _, state := range selection.States {
		kind := Receive
		if state.Dir == types.SendOnly {
			kind = Send
			if state.Send == nil || !scalarType(state.Send.Type()) {
				return "protocol-payload-unknown"
			}
		} else if state.Dir != types.RecvOnly {
			return "protocol-select-alternatives-unknown"
		}
		resource, ok := engine.reference(state.Chan)
		if !ok {
			return "protocol-channel-identity-unknown"
		}
		choice.Arms = append(choice.Arms, SelectArm{Operation: Operation{
			Kind: kind, Resource: resource, Source: state.Pos, Site: selection.Pos(),
		}})
	}
	if !selection.Blocking {
		choice.Arms = append(choice.Arms, SelectArm{Default: true})
	}
	result.Choices = append(result.Choices, choice)
	return "protocol-select-alternatives"
}

func completeChoice(choice SelectChoice) bool {
	if len(choice.Arms) == 0 {
		return false
	}
	for _, arm := range choice.Arms {
		if !arm.Complete {
			return false
		}
	}
	return true
}

// collectSelectFunction follows each outcome of one entry-block select through
// its SSA dispatch and normal return. Any other condition, cycle, unknown
// instruction, or unproven arm keeps the entire choice out of graph proofs.
func (engine *Engine) collectSelectFunction(function *ssa.Function) (Summary, bool) {
	var selection *ssa.Select
	selectIndex := -1
	for _, block := range function.Blocks {
		for index, instruction := range block.Instrs {
			candidate, ok := instruction.(*ssa.Select)
			if !ok {
				continue
			}
			if selection != nil || block != function.Blocks[0] {
				return Summary{Reason: "protocol-select-alternatives-unknown"}, true
			}
			selection, selectIndex = candidate, index
		}
	}
	if selection == nil {
		return Summary{}, false
	}
	if !trivialRecovery(function) {
		return Summary{Reason: "protocol-control-flow-unknown"}, true
	}
	var prefix Summary
	// The select must be reached after one fully accounted-for prefix. We do
	// not merge unrelated branches into this conditional summary.
	for _, instruction := range function.Blocks[0].Instrs[:selectIndex] {
		if !engine.budget.Spend() {
			return Summary{Reason: "protocol-budget-exhausted"}, true
		}
		if reason := engine.appendInstruction(&prefix, instruction, false); reason != "" {
			return Summary{Reason: reason}, true
		}
		if prefix.operationCount() > maxOperations {
			return Summary{Reason: "protocol-summary-limit"}, true
		}
	}
	if !engine.budget.Spend() {
		return Summary{Reason: "protocol-budget-exhausted"}, true
	}
	if reason := engine.appendSelect(&prefix, selection); reason != "protocol-select-alternatives" {
		return Summary{Reason: reason}, true
	}
	choice := &prefix.Choices[0]
	// Analyze every index the Go select may return, including -1 for default.
	// A single failed arm invalidates the whole exhaustive choice proof.
	for index := range choice.Arms {
		state, reason := engine.collectSelectArm(function.Blocks[0], selectIndex+1, selection, index, prefix)
		if reason != "" {
			return Summary{Reason: reason}, true
		}
		choice.Arms[index].Sequence = state.Operations
		choice.Arms[index].Complete = true
	}
	prefix.Reason = "protocol-select-alternatives"
	prefix.AlternativesComplete = true
	return prefix, true
}

func (engine *Engine) collectSelectArm(
	entry *ssa.BasicBlock, afterSelect int, selection *ssa.Select, selected int, prefix Summary,
) (Summary, string) {
	state := cloneEffects(prefix)
	state.Choices = nil
	arm := prefix.Choices[0].Arms[selected]
	if !arm.Default {
		state.Operations = append(state.Operations, arm.Operation)
	}
	block, start := entry, afterSelect
	visited := make(map[*ssa.BasicBlock]bool)
	for {
		// A repeated block would turn one syntactic arm into unbounded loop
		// multiplicity; a fixed event sequence cannot represent that soundly.
		if visited[block] {
			return Summary{}, "protocol-control-flow-unknown"
		}
		visited[block] = true
		for _, instruction := range block.Instrs[start:] {
			if !engine.budget.Spend() {
				return Summary{}, "protocol-budget-exhausted"
			}
			if extract, ok := instruction.(*ssa.Extract); ok && extract.Tuple == selection && scalarType(extract.Type()) {
				continue
			}
			if reason := engine.appendInstruction(&state, instruction, false); reason != "" {
				return Summary{}, reason
			}
			if state.operationCount() > maxOperations {
				return Summary{}, "protocol-summary-limit"
			}
		}
		if len(block.Succs) == 0 {
			if len(state.deferred) != 0 || len(block.Instrs) == 0 {
				return Summary{}, "protocol-deferred-effects-unknown"
			}
			if _, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Return); !ok {
				return Summary{}, "protocol-control-flow-unknown"
			}
			return state, ""
		}
		next, ok := selectSuccessor(block, selection, selected)
		// Only dispatch decisions on the selected index are evaluated here.
		// An independent condition could hide an escaping continuation.
		if !ok {
			return Summary{}, "protocol-select-dispatch-unknown"
		}
		block, start = next, 0
	}
}

func selectSuccessor(block *ssa.BasicBlock, selection *ssa.Select, selected int) (*ssa.BasicBlock, bool) {
	if len(block.Succs) == 1 {
		return block.Succs[0], true
	}
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return nil, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return nil, false
	}
	taken, ok := selectedIndexCondition(branch.Cond, selection, selected)
	if !ok {
		return nil, false
	}
	if taken {
		return block.Succs[0], true
	}
	return block.Succs[1], true
}

func selectedIndexCondition(condition ssa.Value, selection *ssa.Select, selected int) (bool, bool) {
	comparison, ok := condition.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	extract, number := comparison.X, comparison.Y
	if _, ok := extract.(*ssa.Extract); !ok {
		extract, number = number, extract
	}
	index, ok := extract.(*ssa.Extract)
	if !ok || index.Tuple != selection || index.Index != 0 {
		return false, false
	}
	literal, ok := number.(*ssa.Const)
	if !ok || literal.Value == nil {
		return false, false
	}
	want, exact := constant.Int64Val(literal.Value)
	if !exact {
		return false, false
	}
	chosen := selected
	if selected == len(selection.States) {
		chosen = -1
	}
	equal := int64(chosen) == want
	return equal == (comparison.Op == token.EQL), true
}
