package general

import (
	"fmt"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type exitFlowState struct {
	block    *ssa.BasicBlock
	deferred bool
}

func exitPolicyAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "exitpolicy",
		Doc:      "checks process termination that bypasses registered defers",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runExitPolicy,
	}
}

func runExitPolicy(pass *analysis.Pass) (any, error) {
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
		reportExitAfterDefer(pass, function)
	}
	return nil, nil
}

func reportExitAfterDefer(pass *analysis.Pass, function *ssa.Function) {
	if len(function.Blocks) == 0 {
		return
	}
	queue := []exitFlowState{{block: function.Blocks[0]}}
	seen := map[string]bool{}
	reported := map[ssa.Instruction]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := fmt.Sprintf("%d:%t", state.block.Index, state.deferred)
		if seen[key] {
			continue
		}
		seen[key] = true
		deferred := state.deferred
		for _, instruction := range state.block.Instrs {
			if _, ok := instruction.(*ssa.Defer); ok {
				deferred = true
				continue
			}
			common := analysisutil.InstructionCall(instruction)
			if deferred && exitsWithoutRunningDefers(common) && !reported[instruction] {
				reported[instruction] = true
				analysisutil.Reportf(pass, instruction.Pos(), "%s.%s exits without running an earlier defer", shortPackage(analysisutil.CallPackage(common)), analysisutil.CallName(common))
			}
		}
		for _, successor := range state.block.Succs {
			queue = append(queue, exitFlowState{block: successor, deferred: deferred})
		}
	}
}

func exitsWithoutRunningDefers(common *ssa.CallCommon) bool {
	if common == nil {
		return false
	}
	packagePath, name := analysisutil.CallPackage(common), analysisutil.CallName(common)
	if packagePath == "os" && name == "Exit" {
		return true
	}
	if packagePath != "log" {
		return false
	}
	switch name {
	case "Fatal", "Fatalf", "Fatalln":
		return true
	default:
		return false
	}
}
