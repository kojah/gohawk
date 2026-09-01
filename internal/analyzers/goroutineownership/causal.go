package goroutineownership

import (
	"go/token"
	"go/types"
	"strings"

	ssautil "github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/ssa"
)

func causalTestJoin(spawn *ssa.Go, candidate ssa.Instruction) bool {
	common := ssautil.InstructionCall(candidate)
	if common == nil {
		return false
	}
	callee := common.StaticCallee()
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	spawned, _ := closure.Fn.(*ssa.Function)
	if callee == nil || spawned == nil || !functionMayBlock(callee) {
		return false
	}
	if !closurePerformsLifecycleAction(spawned) {
		return false
	}
	for index := range spawned.FreeVars {
		if index >= len(closure.Bindings) {
			continue
		}
		binding := ssautil.CapturedBindingValue(closure.Bindings[index])
		if relatedValue(ssautil.CallReceiver(common), binding) {
			return true
		}
		for _, argument := range common.Args {
			if relatedValue(argument, binding) {
				return true
			}
		}
		if candidateClosure, ok := common.Value.(*ssa.MakeClosure); ok {
			for _, candidateBinding := range candidateClosure.Bindings {
				if relatedValue(ssautil.CapturedBindingValue(candidateBinding), binding) {
					return true
				}
			}
		}
	}
	return false
}

func closurePerformsLifecycleAction(function *ssa.Function) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssautil.InstructionCall(instruction)
			name := strings.ToLower(ssautil.CallName(common))
			if common != nil && (name == "close" || name == "done" || name == "release" || name == "stop" || name == "unlock") {
				return true
			}
		}
	}
	return false
}

func relatedValue(first, second ssa.Value) bool {
	return ssautil.AliasesValue(first, second) || ssautil.ValueDerivesFrom(first, second, map[ssa.Value]bool{}) || ssautil.ValueDerivesFrom(second, first, map[ssa.Value]bool{})
}

func functionMayBlock(function *ssa.Function) bool {
	return functionMayBlockSeen(function, map[*ssa.Function]bool{})
}

func functionMayBlockSeen(function *ssa.Function, seen map[*ssa.Function]bool) bool {
	if function == nil || seen[function] {
		return false
	}
	seen[function] = true
	for _, block := range function.Blocks {
		if blockInCycle(block) {
			return true
		}
		for _, instruction := range block.Instrs {
			if common := ssautil.InstructionCall(instruction); common != nil && functionMayBlockSeen(common.StaticCallee(), seen) {
				return true
			}
			switch typed := instruction.(type) {
			case *ssa.UnOp:
				if typed.Op == token.ARROW {
					return true
				}
			case *ssa.Select:
				for _, state := range typed.States {
					if state.Dir == types.RecvOnly {
						return true
					}
				}
			}
		}
	}
	return false
}
