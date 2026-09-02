// Package exitpolicy implements the exitpolicy gohawk analyzer.
package exitpolicy

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type exitFlowState struct {
	block    *ssa.BasicBlock
	deferred bool
}

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "exitpolicy",
		Doc:      "checks process termination that bypasses registered defers",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runExitPolicy,
	}
}

func runExitPolicy(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportExitAfterDefer(pass, function)
	}
	return nil, nil
}

func reportExitAfterDefer(pass *analysis.Pass, function *ssa.Function) {
	if len(function.Blocks) == 0 {
		return
	}
	reported := map[ssa.Instruction]bool{}
	identity := func(state exitFlowState) exitFlowState { return state }
	ssaflow.WalkStates([]exitFlowState{{block: function.Blocks[0]}}, identity, func(state exitFlowState) ([]exitFlowState, bool) {
		deferred := state.deferred
		for _, instruction := range state.block.Instrs {
			if _, ok := instruction.(*ssa.Defer); ok {
				deferred = deferred || processExitRelevantDefer(ssaflow.InstructionCall(instruction))
				continue
			}
			common := ssaflow.InstructionCall(instruction)
			if deferred && exitsWithoutRunningDefers(common) && !reported[instruction] {
				reported[instruction] = true
				check.Reportf(
					pass,
					check.ExitSkipsDefer,
					instruction.Pos(),
					"%s.%s exits without running an earlier defer",
					syntax.ShortPackageName(ssaflow.CallPackage(common)),
					ssaflow.CallName(common),
				)
			}
		}
		successors := make([]exitFlowState, 0, len(state.block.Succs))
		for _, successor := range state.block.Succs {
			successors = append(successors, exitFlowState{block: successor, deferred: deferred})
		}
		return successors, true
	})
}

func processExitRelevantDefer(common *ssa.CallCommon) bool {
	if common == nil || common.Value == nil {
		return true
	}
	// Canceling an in-process context cannot release an external resource once
	// the process is already terminating. Treating a deferred CancelFunc as
	// meaningful cleanup made startup fatal paths noisy without identifying a
	// lost flush or close. ccLoad initializes bounded startup contexts this way:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/server.go#L166-L199
	return !syntax.NamedType(common.Value.Type(), "context", "CancelFunc")
}

func exitsWithoutRunningDefers(common *ssa.CallCommon) bool {
	for _, symbol := range []syntax.Symbol{
		syntax.PackageFunction("os", "Exit"),
		syntax.PackageFunction("log", "Fatal"),
		syntax.PackageFunction("log", "Fatalf"),
		syntax.PackageFunction("log", "Fatalln"),
	} {
		if ssaflow.CallMatchesSymbol(common, symbol) {
			return true
		}
	}
	return false
}
