// Package exitpolicy implements the exitpolicy gohawk analyzer.
package exitpolicy

import (
	"fmt"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/analyzerbase"

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
	functions, err := ssautil.SourceSSAFunctions(pass)
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
				deferred = deferred || processExitRelevantDefer(ssautil.InstructionCall(instruction))
				continue
			}
			common := ssautil.InstructionCall(instruction)
			if deferred && exitsWithoutRunningDefers(common) && !reported[instruction] {
				reported[instruction] = true
				analyzerbase.Reportf(pass, analyzerbase.CheckExitSkipsDefer, instruction.Pos(), "%s.%s exits without running an earlier defer", analysisutil.ShortPackageName(ssautil.CallPackage(common)), ssautil.CallName(common))
			}
		}
		for _, successor := range state.block.Succs {
			queue = append(queue, exitFlowState{block: successor, deferred: deferred})
		}
	}
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
	return !analysisutil.NamedType(common.Value.Type(), "context", "CancelFunc")
}

func exitsWithoutRunningDefers(common *ssa.CallCommon) bool {
	if common == nil {
		return false
	}
	packagePath, name := ssautil.CallPackage(common), ssautil.CallName(common)
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
