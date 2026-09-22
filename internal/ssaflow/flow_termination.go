package ssaflow

import "golang.org/x/tools/go/ssa"

// InstructionTerminatesControlFlow reports calls whose documented behavior
// prevents execution from continuing in the current goroutine.
func InstructionTerminatesControlFlow(instruction ssa.Instruction) bool {
	if call, ok := instruction.(*ssa.Call); ok {
		return callTerminatesControlFlow(call.Common())
	}
	if _, ok := instruction.(*ssa.RunDefers); !ok {
		return false
	}
	// Registration does not terminate execution. At RunDefers, require a
	// registration on every path; a conditional defer alone is insufficient.
	// Process-owned descriptors can intentionally live until this exit.
	// https://github.com/golang/sys/blob/01b91195d9aeaba1dab70b882a12f741f568a510/unix/syscall_unix_test.go#L274
	for _, deferred := range InstructionsOf[*ssa.Defer](instruction.Parent()) {
		if callTerminatesControlFlow(deferred.Common()) && InstructionDominates(deferred, instruction) {
			return true
		}
	}
	return false
}

func callTerminatesControlFlow(common *ssa.CallCommon) bool {
	return HasLibraryContract(common, ContractRuntimeGoexit) || HasLibraryContract(common, ContractTestingTermination) ||
		HasLibraryContract(common, ContractProcessExit)
}
