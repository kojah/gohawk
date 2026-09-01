package ssaflow

import "golang.org/x/tools/go/ssa"

// StaticCallsites indexes every statically resolved call-like instruction by
// its callee.
func StaticCallsites(functions []*ssa.Function) map[*ssa.Function][]ssa.CallInstruction {
	result := make(map[*ssa.Function][]ssa.CallInstruction)
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(ssa.CallInstruction)
				if !ok || call.Common().StaticCallee() == nil {
					continue
				}
				callee := call.Common().StaticCallee()
				result[callee] = append(result[callee], call)
			}
		}
	}
	return result
}

// StaticCalls indexes ordinary statically resolved calls by their callee. It
// deliberately excludes deferred and goroutine calls.
func StaticCalls(functions []*ssa.Function) map[*ssa.Function][]*ssa.Call {
	result := make(map[*ssa.Function][]*ssa.Call)
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok || call.Common().StaticCallee() == nil {
					continue
				}
				callee := call.Common().StaticCallee()
				result[callee] = append(result[callee], call)
			}
		}
	}
	return result
}
