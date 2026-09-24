package concurrencyfacts

import (
	"fmt"
	"go/token"
	"strconv"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Cutoff provenance is local diagnostic metadata, never evidence or an exported
// fact. Cached summaries retain the leaf; composition copies the bounded chain
// so another caller cannot change an earlier caller's explanation.
const maxCutoffCalls = 8

type cutoffShape uint8

const (
	cutoffUnspecified cutoffShape = iota
	cutoffInstruction
	cutoffLoop
	cutoffRecovery
	cutoffSelect
	cutoffBranch
)

func (shape cutoffShape) String() string {
	switch shape {
	case cutoffInstruction:
		return "instruction"
	case cutoffLoop:
		return "unsupported-loop-or-branch"
	case cutoffRecovery:
		return "recovery"
	case cutoffSelect:
		return "select-dispatch-or-alternatives"
	case cutoffBranch:
		return "branch-merge-or-limit"
	default:
		return "unspecified"
	}
}

type summaryCutoff struct {
	instruction ssa.Instruction
	function    *ssa.Function
	shape       cutoffShape
	calls       [maxCutoffCalls]ssa.CallInstruction
	depth       int
	truncated   bool
}

func (engine *Engine) recordCutoff(instruction ssa.Instruction, shape cutoffShape) {
	if engine.cutoff == nil && instruction != nil {
		engine.cutoff = &summaryCutoff{instruction: instruction, function: instruction.Parent(), shape: shape}
	}
}

func (engine *Engine) recordBlockCutoff(block *ssa.BasicBlock, shape cutoffShape) {
	if block != nil && len(block.Instrs) != 0 {
		engine.recordCutoff(block.Instrs[len(block.Instrs)-1], shape)
	}
}

func (engine *Engine) instantiatedCutoff(result Summary, call ssa.CallInstruction) Summary {
	if result.Reason == ReasonNone || result.AlternativesComplete || len(result.Paths) != 0 {
		return result
	}
	cutoff := summaryCutoff{instruction: call, function: call.Parent(), shape: cutoffInstruction}
	if result.cutoff != nil {
		cutoff = *result.cutoff
		if cutoff.depth < len(cutoff.calls) {
			cutoff.calls[cutoff.depth] = call
			cutoff.depth++
		} else {
			cutoff.truncated = true
		}
	}
	result.cutoff = &cutoff
	engine.cutoff = result.cutoff
	return result
}

// ObserveCutoff emits the retained summary cutoff through a candidate's observer.
// It never reruns inference. A nil observer does no formatting or allocation.
// Positions and SSA text are developer-local evidence, not serialized facts.
// The call chain runs from the leaf outward and is explicitly marked if cut.
func (summary Summary) ObserveCutoff(observer ssaflow.Observer) {
	if observer == nil || summary.cutoff == nil || summary.Reason == ReasonNone {
		return
	}
	cutoff := summary.cutoff
	details := map[string]string{
		"summary-reason": summary.Reason.String(), "shape": cutoff.shape.String(),
		"call-depth": strconv.Itoa(cutoff.depth), "chain-truncated": strconv.FormatBool(cutoff.truncated),
	}
	at := token.NoPos
	if cutoff.function != nil {
		details["function"] = cutoff.function.String()
		at = cutoff.function.Pos()
	}
	if cutoff.instruction != nil {
		if cutoff.instruction.Pos().IsValid() {
			at = cutoff.instruction.Pos()
		}
		details["instruction"] = cutoff.instruction.String()
		details["instruction-kind"] = fmt.Sprintf("%T", cutoff.instruction)
	}
	for index, call := range cutoff.calls[:cutoff.depth] {
		key := "caller-" + strconv.Itoa(index)
		details[key] = call.Parent().String()
		if program := call.Parent().Prog; program != nil {
			details[key+"-position"] = program.Fset.Position(call.Pos()).String()
		}
	}
	observer(ReasonCutoff.String(), at, details)
}
