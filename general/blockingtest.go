package general

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/checkutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func blockingTestAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "blockingtest",
		Doc:      "checks cancellation ownership for blocking test channels",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runBlockingTest,
	}
}

func runBlockingTest(pass *analysis.Pass) (any, error) {
	for _, function := range checkutil.SourceSSAFunctions(pass) {
		file := checkutil.FunctionFile(pass, function)
		if file == nil || !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		contextAware := ssaFunctionHasContext(function)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				switch typed := instruction.(type) {
				case *ssa.Send:
					if contextAware {
						pass.Reportf(typed.Pos(), "channel send in context-aware test code requires cancellation-aware select")
					}
				case *ssa.UnOp:
					if typed.Op == token.ARROW && !cancellationChannel(typed.X) && !timerChannel(typed.X) && !closedBefore(function, typed.X, typed) {
						pass.Reportf(typed.Pos(), "blocking channel receive in test code requires cancellation-aware select")
					}
				case *ssa.Select:
					if typed.Blocking && !selectHasCancellation(typed) {
						pass.Reportf(typed.Pos(), "blocking channel select in test code requires cancellation escape")
					}
				}
			}
		}
	}
	return nil, nil
}

func closedBefore(function *ssa.Function, channel ssa.Value, receive ssa.Instruction) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := checkutil.InstructionCall(instruction)
			if common == nil || checkutil.CallName(common) != checkutil.BuiltinClose || len(common.Args) != 1 || !checkutil.AliasesValue(common.Args[0], channel) {
				continue
			}
			if block != receive.Block() && block.Dominates(receive.Block()) {
				return true
			}
			if block == receive.Block() && checkutil.InstructionIndex(instruction) < checkutil.InstructionIndex(receive) {
				return true
			}
		}
	}
	return false
}

func ssaFunctionHasContext(function *ssa.Function) bool {
	for _, parameter := range function.Params {
		if isContext(parameter.Type()) {
			return true
		}
	}
	for _, free := range function.FreeVars {
		if isContext(free.Type()) {
			return true
		}
	}
	return false
}

func selectHasCancellation(selection *ssa.Select) bool {
	for _, state := range selection.States {
		if state.Dir == types.RecvOnly && (cancellationChannel(state.Chan) || timerChannel(state.Chan)) {
			return true
		}
	}
	return false
}

func cancellationChannel(value ssa.Value) bool {
	return valueGraphHasCall(value, func(common *ssa.CallCommon) bool {
		receiver := checkutil.CallReceiver(common)
		return checkutil.CallName(common) == "Done" && receiver != nil && isContext(receiver.Type())
	}, map[ssa.Value]bool{})
}

func valueGraphHasCall(value ssa.Value, match func(*ssa.CallCommon) bool, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if call, ok := value.(*ssa.Call); ok && match(call.Common()) {
		return true
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil && valueGraphHasCall(*operand, match, seen) {
			return true
		}
	}
	return false
}

func timerChannel(value ssa.Value) bool {
	channel, ok := value.Type().Underlying().(*types.Chan)
	return ok && checkutil.NamedType(channel.Elem(), "time", "Time")
}
