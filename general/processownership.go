package general

import (
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/checkutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func processOwnershipAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "processownership",
		Doc:      "checks that started os/exec commands have a wait owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runProcessOwnership,
	}
}

func runProcessOwnership(pass *analysis.Pass) (any, error) {
	for _, function := range checkutil.SourceSSAFunctions(pass) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				start, ok := instruction.(*ssa.Call)
				if !ok || checkutil.CallName(start.Common()) != "Start" || !execCommandValue(checkutil.CallReceiver(start.Common())) {
					continue
				}
				command := checkutil.CallReceiver(start.Common())
				// Caller retains a parameter command after this helper returns, so
				// helper-local Start does not transfer caller's Wait responsibility.
				if aliasesAny(command, parameterValues(function.Params)) {
					continue
				}
				if checkutil.UnownedReturn(start, func(candidate ssa.Instruction) bool {
					common := checkutil.InstructionCall(candidate)
					return waitsForCommand(candidate, command) || checkutil.DeferredClosureCalls(candidate, "Wait", command) || checkutil.CallPackage(common) == "os" && checkutil.CallName(common) == "Exit"
				}, func(returned *ssa.Return) bool {
					return startFailureReturn(returned, start)
				}) {
					pass.Reportf(start.Pos(), "started command is not waited on every successful return path")
				}
			}
		}
	}
	return nil, nil
}

func parameterValues(parameters []*ssa.Parameter) []ssa.Value {
	values := make([]ssa.Value, len(parameters))
	for index, parameter := range parameters {
		values[index] = parameter
	}
	return values
}

func execCommandValue(value ssa.Value) bool {
	if value == nil {
		return false
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	return ok && checkutil.NamedType(pointer.Elem(), "os/exec", "Cmd")
}

func waitsForCommand(instruction ssa.Instruction, command ssa.Value) bool {
	common := checkutil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if checkutil.CallName(common) == "Wait" && checkutil.AliasesValue(checkutil.CallReceiver(common), command) {
		return true
	}
	lower := strings.ToLower(checkutil.CallName(common))
	if !strings.Contains(lower, "wait") && !strings.Contains(lower, "join") {
		return false
	}
	for _, argument := range common.Args {
		if checkutil.AliasesValue(argument, command) {
			return true
		}
	}
	return false
}

func startFailureReturn(returned *ssa.Return, start *ssa.Call) bool {
	if returned.Block() == start.Block() {
		return false
	}
	for _, predecessor := range returned.Block().Preds {
		instructions := predecessor.Instrs
		if len(instructions) == 0 {
			continue
		}
		branch, ok := instructions[len(instructions)-1].(*ssa.If)
		if ok && valueDependsOn(branch.Cond, start, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func valueDependsOn(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || target == nil || seen[value] {
		return false
	}
	if value == target {
		return true
	}
	seen[value] = true
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil && valueDependsOn(*operand, target, seen) {
			return true
		}
	}
	return false
}
