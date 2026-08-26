package general

import (
	"strings"

	"github.com/kojah/gohawk/internal/checkutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func cancellationOwnershipAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "cancellationownership",
		Doc:      "checks derived context cancellation functions are called on every return path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runCancellationOwnership,
	}
}

func runCancellationOwnership(pass *analysis.Pass) (any, error) {
	for _, function := range checkutil.SourceSSAFunctions(pass) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok || checkutil.CallPackage(call.Common()) != "context" || !strings.HasPrefix(checkutil.CallName(call.Common()), "With") {
					continue
				}
				cancel := resourceResult(call, 1)
				if cancel == nil {
					continue
				}
				if checkutil.UnownedReturn(call, func(candidate ssa.Instruction) bool {
					return callsCancel(candidate, cancel)
				}, func(returned *ssa.Return) bool {
					return returnedValueAliases(returned, cancel)
				}) {
					pass.Reportf(call.Pos(), "cancel function from context.%s is not called on every return path", checkutil.CallName(call.Common()))
				}
			}
		}
	}
	return nil, nil
}

func callsCancel(instruction ssa.Instruction, cancel ssa.Value) bool {
	common := checkutil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if checkutil.AliasesValue(common.Value, cancel) {
		return true
	}
	name := strings.ToLower(checkutil.CallName(common))
	if !strings.Contains(name, "cancel") && !strings.Contains(name, "stop") {
		return false
	}
	for _, argument := range common.Args {
		if checkutil.AliasesValue(argument, cancel) {
			return true
		}
	}
	return false
}
