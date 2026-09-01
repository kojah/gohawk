package goroutineownership

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

func causalTestJoin(spawn *ssa.Go, candidate ssa.Instruction) bool {
	if causallyJoinedByOwnedWorker(spawn, candidate) {
		return true
	}
	common := ssaflow.InstructionCall(candidate)
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
	// A blocking call is causal join evidence only when it operates on a value
	// captured by the worker that performs the lifecycle action. Mere source
	// order between unrelated blocking work and the spawn is insufficient.
	for index := range spawned.FreeVars {
		if index >= len(closure.Bindings) {
			continue
		}
		binding := ssaflow.CapturedBindingValue(closure.Bindings[index])
		if relatedValue(ssaflow.CallReceiver(common), binding) {
			return true
		}
		for _, argument := range common.Args {
			if relatedValue(argument, binding) {
				return true
			}
		}
		if candidateClosure, ok := common.Value.(*ssa.MakeClosure); ok {
			for _, candidateBinding := range candidateClosure.Bindings {
				if relatedValue(ssaflow.CapturedBindingValue(candidateBinding), binding) {
					return true
				}
			}
		}
	}
	return false
}

func causallyJoinedByOwnedWorker(spawn *ssa.Go, candidate ssa.Instruction) bool {
	// A joined controller worker may shut down the shared blocking owner and
	// thereby make the first worker finite without receiving its own signal.
	// https://github.com/gumieri/nenya/blob/efae2b7519f8ee292dce3b5a86d509aa5a73b257/cmd/nenya/main_test.go#L405-L414
	joined, ok := candidate.(*ssa.Go)
	if !ok {
		return false
	}
	spawned := spawn.Common().StaticCallee()
	if closure, closureOK := spawn.Common().Value.(*ssa.MakeClosure); closureOK {
		spawned, _ = closure.Fn.(*ssa.Function)
	}
	worker := joined.Common().StaticCallee()
	workerClosure, _ := joined.Common().Value.(*ssa.MakeClosure)
	if workerClosure != nil {
		worker, _ = workerClosure.Fn.(*ssa.Function)
	}
	if spawned == nil || worker == nil || !functionMayBlock(spawned) || !closurePerformsLifecycleAction(worker) {
		return false
	}
	signals, groups := goroutineJoinValues(joined)
	if len(signals)+len(groups) == 0 || ssaflow.UnownedReturn(joined, func(next ssa.Instruction) bool {
		return joinsGoroutine(next, signals, groups)
	}, nil) {
		return false
	}
	spawnValues := append([]ssa.Value{}, spawn.Common().Args...)
	if closure, closureOK := spawn.Common().Value.(*ssa.MakeClosure); closureOK {
		spawnValues = append(spawnValues, closure.Bindings...)
	}
	workerValues := append([]ssa.Value{}, joined.Common().Args...)
	if workerClosure != nil {
		workerValues = append(workerValues, workerClosure.Bindings...)
	}
	for _, first := range spawnValues {
		for _, second := range workerValues {
			if relatedValue(ssaflow.CapturedBindingValue(first), ssaflow.CapturedBindingValue(second)) {
				return true
			}
		}
	}
	return false
}

func closurePerformsLifecycleAction(function *ssa.Function) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			name := strings.ToLower(ssaflow.CallName(common))
			if common != nil && (name == "close" || name == "done" || name == "release" || name == "stop" || name == "unlock") {
				return true
			}
		}
	}
	return false
}

func relatedValue(first, second ssa.Value) bool {
	return ssaflow.SameValue(first, second) || ssaflow.ValueDerivesFrom(first, second, map[ssa.Value]bool{}) ||
		ssaflow.ValueDerivesFrom(second, first, map[ssa.Value]bool{})
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
		if ssaflow.BlockInCycle(block) {
			return true
		}
		for _, instruction := range block.Instrs {
			if common := ssaflow.InstructionCall(instruction); common != nil && functionMayBlockSeen(common.StaticCallee(), seen) {
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
