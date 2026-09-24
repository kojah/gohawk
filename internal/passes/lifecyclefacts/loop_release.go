package lifecyclefacts

import (
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A helper may release every element of the aggregate it receives inside a
// loop, as client-go's CloseAndRemove closes each of its variadic files:
// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/staging/src/k8s.io/client-go/util/testing/remove_file.go#L25-L39
// No every-return mask can carry that: the loop's exit edge skips the body,
// and which element an iteration releases is decided by iteration rather
// than by the path. The summary records the loop as a separate may-claim so
// an importing caller can treat the call as uncertain instead of untouched.
// It is never evidence that any particular element was released.

// releasesDerivedValueInLoop reports whether the function calls a lifecycle
// cleanup method, inside a block in a cycle, on a value derived from the
// parameter.
func releasesDerivedValueInLoop(function *ssa.Function, parameter ssa.Value) bool {
	for _, block := range function.Blocks {
		if blockReleasesDerivedValue(block, parameter) && ssaflow.BlockInCycle(block) {
			return true
		}
	}
	return false
}

func blockReleasesDerivedValue(block *ssa.BasicBlock, parameter ssa.Value) bool {
	for _, instruction := range block.Instrs {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			continue
		}
		name := ssaflow.CallName(common)
		if slices.ContainsFunc(lifecycleMasks, func(mask lifecycleMask) bool { return mask.method != "" && mask.method == name }) &&
			heapmodel.ValueDerivesFrom(ssaflow.CallReceiver(common), parameter, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}
