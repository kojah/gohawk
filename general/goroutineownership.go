package general

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func goroutineOwnershipAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "goroutineownership",
		Doc:      "checks that explicit goroutines have a recognizable join owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runGoroutineOwnership,
	}
}

func runGoroutineOwnership(pass *analysis.Pass) (any, error) {
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				spawn, ok := instruction.(*ssa.Go)
				if !ok {
					continue
				}
				signals, groups := goroutineJoinValues(spawn)
				if analysisutil.UnownedReturn(spawn, func(candidate ssa.Instruction) bool {
					return joinsGoroutine(candidate, signals, groups)
				}, nil) {
					pass.Reportf(spawn.Pos(), "goroutine is not joined on every return path")
				}
			}
		}
	}
	return nil, nil
}

func goroutineJoinValues(spawn *ssa.Go) (signals, groups []ssa.Value) {
	function, closure, ok := spawnedClosure(spawn)
	if !ok {
		return nil, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			signal, group := closureOwnershipValue(function, closure, instruction)
			if signal != nil {
				signals = append(signals, signal)
			}
			if group != nil {
				groups = append(groups, group)
			}
		}
	}
	return signals, groups
}

func spawnedClosure(spawn *ssa.Go) (*ssa.Function, *ssa.MakeClosure, bool) {
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return nil, nil, false
	}
	function, ok := closure.Fn.(*ssa.Function)
	return function, closure, ok
}

func closureOwnershipValue(function *ssa.Function, closure *ssa.MakeClosure, instruction ssa.Instruction) (signal, group ssa.Value) { //nolint:ireturn // Closure ownership can flow through channels or synchronization values.
	if send, ok := instruction.(*ssa.Send); ok {
		return closureBinding(function, closure, send.Chan), nil
	}
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return nil, nil
	}
	switch analysisutil.CallName(common) {
	case analysisutil.BuiltinClose:
		if len(common.Args) == 1 {
			return closureBinding(function, closure, common.Args[0]), nil
		}
	case "Done":
		return nil, closureBinding(function, closure, analysisutil.CallReceiver(common))
	}
	return nil, nil
}

func closureBinding(function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value) ssa.Value { //nolint:ireturn // Captures retain their concrete SSA value form.
	for index, free := range function.FreeVars {
		if analysisutil.AliasesValue(value, free) && index < len(closure.Bindings) {
			return analysisutil.CapturedBindingValue(closure.Bindings[index])
		}
	}
	return nil
}

func joinsGoroutine(instruction ssa.Instruction, signals, groups []ssa.Value) bool {
	if receive, ok := instruction.(*ssa.UnOp); ok && receive.Op == token.ARROW {
		return aliasesAny(receive.X, signals)
	}
	if selection, ok := instruction.(*ssa.Select); ok {
		for _, state := range selection.States {
			if state.Dir == types.RecvOnly && aliasesAny(state.Chan, signals) {
				return true
			}
		}
	}
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if analysisutil.CallName(common) == "Wait" && aliasesAny(analysisutil.CallReceiver(common), groups) {
		return true
	}
	lower := strings.ToLower(analysisutil.CallName(common))
	if !strings.Contains(lower, "wait") && !strings.Contains(lower, "join") {
		return false
	}
	for _, argument := range common.Args {
		if aliasesAny(argument, signals) || aliasesAny(argument, groups) {
			return true
		}
	}
	return false
}

func aliasesAny(value ssa.Value, targets []ssa.Value) bool {
	for _, target := range targets {
		if analysisutil.AliasesValue(value, target) {
			return true
		}
	}
	return false
}
