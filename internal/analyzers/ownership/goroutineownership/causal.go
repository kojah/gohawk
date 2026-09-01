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
	// A blocking call is causal join evidence only when it operates on a value
	// captured by the worker and the worker's lifecycle action operates on that
	// exact capture. Mere source order, or cleanup of a different captured value,
	// is insufficient.
	for index := range spawned.FreeVars {
		if index >= len(closure.Bindings) {
			continue
		}
		if !functionDirectlyPerformsLifecycleActionOn(spawned, spawned.FreeVars[index]) {
			continue
		}
		binding := ssaflow.CapturedBindingValue(closure.Bindings[index])
		if callUsesRelatedValue(common, binding) {
			return true
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
	if spawned == nil || worker == nil || !functionMayBlock(spawned) {
		return false
	}
	signals, groups, _ := goroutineJoinValues(joined)
	if len(signals)+len(groups) == 0 || ssaflow.UnownedReturn(joined, func(next ssa.Instruction) bool {
		return joinsGoroutine(next, signals, groups)
	}, nil) {
		return false
	}
	spawnValues := append([]ssa.Value{}, spawn.Common().Args...)
	if closure, closureOK := spawn.Common().Value.(*ssa.MakeClosure); closureOK {
		spawnValues = append(spawnValues, closure.Bindings...)
	}
	workerValues := suppliedWorkerValues(joined, worker, workerClosure)
	for _, first := range spawnValues {
		for _, second := range workerValues {
			if functionPerformsLifecycleActionOn(worker, second.local, map[*ssa.Function]bool{}) &&
				relatedValue(ssaflow.CapturedBindingValue(first), ssaflow.CapturedBindingValue(second.supplied)) {
				return true
			}
		}
	}
	return false
}

func functionDirectlyPerformsLifecycleActionOn(function *ssa.Function, target ssa.Value) bool {
	if function == nil || target == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if lifecycleActionUsesValue(ssaflow.InstructionCall(instruction), target) {
				return true
			}
		}
	}
	return false
}

type suppliedWorkerValue struct {
	local    ssa.Value
	supplied ssa.Value
}

func suppliedWorkerValues(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure) []suppliedWorkerValue {
	values := make([]suppliedWorkerValue, 0, len(function.Params)+len(function.FreeVars))
	for index, parameter := range function.Params {
		if index < len(spawn.Common().Args) {
			values = append(values, suppliedWorkerValue{local: parameter, supplied: spawn.Common().Args[index]})
		}
	}
	if closure == nil {
		return values
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) {
			values = append(values, suppliedWorkerValue{local: free, supplied: closure.Bindings[index]})
		}
	}
	return values
}

// Lifecycle evidence may cross a source-visible helper only through the exact
// parameter supplied from the candidate owner. This keeps Nenya's
// eventLoop(..., srv) -> srv.Shutdown(...) chain while rejecting a helper that
// cleans a different captured object.
func functionPerformsLifecycleActionOn(function *ssa.Function, target ssa.Value, seen map[*ssa.Function]bool) bool {
	if function == nil || target == nil || seen[function] {
		return false
	}
	seen[function] = true
	defer delete(seen, function)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if lifecycleActionUsesValue(common, target) {
				return true
			}
			if common == nil {
				continue
			}
			callee := common.StaticCallee()
			if callee == nil || len(callee.Blocks) == 0 {
				continue
			}
			for index, argument := range common.Args {
				if index < len(callee.Params) && relatedValue(argument, target) &&
					functionPerformsLifecycleActionOn(callee, callee.Params[index], seen) {
					return true
				}
			}
		}
	}
	return false
}

func lifecycleActionUsesValue(common *ssa.CallCommon, target ssa.Value) bool {
	if common == nil {
		return false
	}
	switch strings.ToLower(ssaflow.CallName(common)) {
	case "close", "done", "release", "stop", "unlock":
	default:
		return false
	}
	return callUsesRelatedValue(common, target)
}

func callUsesRelatedValue(common *ssa.CallCommon, target ssa.Value) bool {
	if common == nil || target == nil {
		return false
	}
	if relatedValue(ssaflow.CallReceiver(common), target) {
		return true
	}
	for _, argument := range common.Args {
		if relatedValue(argument, target) {
			return true
		}
	}
	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		for _, binding := range closure.Bindings {
			if relatedValue(ssaflow.CapturedBindingValue(binding), target) {
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
