package ssaflow

import "golang.org/x/tools/go/ssa"

// Terminator extends the documented catalog of terminating calls with what
// an analyzer knows from summaries: a project's own fatal wrapper, or a
// server loop that never returns. It reports only calls; the catalog still
// decides deferred exits and runtime.Goexit.
type Terminator func(*ssa.Call) bool

// InstructionTerminatesControlFlow reports calls whose documented behavior
// prevents execution from continuing in the current goroutine.
func InstructionTerminatesControlFlow(instruction ssa.Instruction) bool {
	return InstructionTerminatesWith(instruction, nil)
}

// InstructionTerminatesWith is InstructionTerminatesControlFlow extended by
// a terminator; a nil terminator leaves the catalog alone.
func InstructionTerminatesWith(instruction ssa.Instruction, terminates Terminator) bool {
	if call, ok := instruction.(*ssa.Call); ok {
		return callTerminatesControlFlow(call.Common()) || terminates != nil && terminates(call)
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
