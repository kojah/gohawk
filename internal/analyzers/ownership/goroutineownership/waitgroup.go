package goroutineownership

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

var waitGroupDone = syntax.PackageMethod(syntax.MethodSymbol{
	PackagePath: "sync",
	Receiver:    "WaitGroup",
	Name:        "Done",
})

var waitGroupWait = syntax.PackageMethod(syntax.MethodSymbol{
	PackagePath: "sync",
	Receiver:    "WaitGroup",
	Name:        "Wait",
})

// WaitGroup completion evidence distinguishes a goroutine's settlement from
// an earlier progress signal. A direct Done must be terminal on every normal
// path; a deferred Done must be registered on every normal path.

func waitGroupCompletionValues(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
) (groups []ssa.Value, unsettled ssa.Instruction) {
	for _, candidate := range waitGroupDoneReceivers(function) {
		if containsSameValue(groups, ssaflow.SpawnedValueAtCall(spawn, function, closure, candidate.receiver)) {
			continue
		}
		if !waitGroupSettlesFunction(function, candidate.receiver) {
			if unsettled == nil {
				unsettled = candidate.instruction
			}
			continue
		}
		group := ssaflow.SpawnedValueAtCall(spawn, function, closure, candidate.receiver)
		if group != nil {
			groups = append(groups, group)
		}
	}
	return groups, unsettled
}

func dominatingDeferredWaitGroupJoin(function *ssa.Function, spawn *ssa.Go, groups []ssa.Value) ssa.Instruction {
	if function == nil || spawn == nil || len(groups) == 0 {
		return nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			deferred, ok := instruction.(*ssa.Defer)
			if !ok || !ssaflow.InstructionDominates(deferred, spawn) {
				continue
			}
			common := deferred.Common()
			if !ssaflow.CallMatchesSymbol(common, waitGroupWait) ||
				!ssaflow.SameAsAny(ssaflow.CallReceiver(common), groups) {
				continue
			}
			// A defer registered before every path to the spawn runs after all
			// subsequently registered workers have called Done. This is the shape
			// used by Zap's pool race test:
			// https://github.com/uber-go/zap/blob/bb1a55dd13257cf7cbd06b4146674c67ca614dea/internal/pool/pool_test.go#L85-L105
			return deferred
		}
	}
	return nil
}

type waitGroupDoneCall struct {
	instruction ssa.Instruction
	receiver    ssa.Value
}

func waitGroupDoneReceivers(function *ssa.Function) []waitGroupDoneCall {
	var result []waitGroupDoneCall
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if common != nil && ssaflow.CallMatchesSymbol(common, waitGroupDone) {
				result = append(result, waitGroupDoneCall{instruction: instruction, receiver: ssaflow.CallReceiver(common)})
			}
		}
	}
	return result
}

func waitGroupSettlesFunction(function *ssa.Function, receiver ssa.Value) bool {
	// Done may announce progress without completing the goroutine. Moov's test
	// calls Done before validation, so Wait can return while the worker still
	// uses the subtest and shared cache:
	// https://github.com/moov-io/rtp20022/blob/0b08f38d0a1341d61a4d1fe7b0a402b5718d3f30/pkg/rtp/restrictions_test.go#L27-L43
	hasReturn := false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
		}
	}
	if !hasReturn {
		return false
	}
	return !ssaflow.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		if common == nil || !ssaflow.CallMatchesSymbol(common, waitGroupDone) ||
			!ssaflow.ValueAliases(ssaflow.CallReceiver(common), receiver, map[ssa.Value]bool{}) {
			return false
		}
		if _, deferred := instruction.(*ssa.Defer); deferred {
			return true
		}
		return waitGroupDoneIsTerminal(instruction)
	})
}

func waitGroupDoneIsTerminal(done ssa.Instruction) bool {
	index := ssaflow.InstructionIndex(done)
	if index < 0 {
		return false
	}
	type cursor struct {
		block *ssa.BasicBlock
		index int
	}
	queue := []cursor{{block: done.Block(), index: index + 1}}
	seen := make(map[cursor]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			return false
		}
		seen[current] = true
		if current.index < len(current.block.Instrs) {
			if _, returned := current.block.Instrs[current.index].(*ssa.Return); !returned {
				return false
			}
			continue
		}
		if len(current.block.Succs) == 0 {
			return false
		}
		for _, successor := range current.block.Succs {
			queue = append(queue, cursor{block: successor})
		}
	}
	return true
}

func containsSameValue(values []ssa.Value, target ssa.Value) bool {
	return target != nil && ssaflow.SameAsAny(target, values)
}
