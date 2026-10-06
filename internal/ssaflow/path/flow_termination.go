package path

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	ssaflow "github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"golang.org/x/tools/go/ssa"
)

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
	return InstructionTerminatesWithin(instruction, terminates, nil)
}

// InstructionTerminatesWithin shares the allowance across call dispatch,
// deferred registration census and dominance. Exhaustion supplies no positive
// termination evidence; callers retain availability before continuing a path.
// A nil budget preserves the default termination policy.
func InstructionTerminatesWithin(instruction ssa.Instruction, terminates Terminator, budget *proofs.SearchBudget) bool {
	if call, ok := instruction.(*ssa.Call); ok {
		if !budget.Spend() {
			return false
		}
		terminatesPath := callTerminatesControlFlow(call.Common()) || terminates != nil && terminates(call)
		return terminatesPath && !budget.Exhausted()
	}
	if _, ok := instruction.(*ssa.RunDefers); !ok {
		return false
	}
	// Registration does not terminate execution. At RunDefers, require a
	// registration on every path; a conditional defer alone is insufficient.
	// Process-owned descriptors can intentionally live until this exit.
	// https://github.com/golang/sys/blob/01b91195d9aeaba1dab70b882a12f741f568a510/unix/syscall_unix_test.go#L274
	for candidate := range ssaflow.InstructionsWithin(instruction.Parent(), budget) {
		deferred, ok := candidate.(*ssa.Defer)
		if !ok || !callTerminatesControlFlow(deferred.Common()) {
			continue
		}
		if cfg.InstructionDominatesWithin(deferred, instruction, budget) {
			return !budget.Exhausted()
		}
		if budget.Exhausted() {
			return false
		}
	}
	return false
}

func callTerminatesControlFlow(common *ssa.CallCommon) bool {
	return ssacall.HasLibraryContract(common, ssacall.ContractRuntimeGoexit) || ssacall.HasLibraryContract(common, ssacall.ContractTestingTermination) ||
		ssacall.HasLibraryContract(common, ssacall.ContractProcessExit)
}

// BlockEndsInPanic reports whether the final instruction is an explicit panic.
// It does not establish function termination: deferred recovery may return.
func BlockEndsInPanic(block *ssa.BasicBlock) bool {
	if block == nil || len(block.Instrs) == 0 {
		return false
	}
	_, panics := block.Instrs[len(block.Instrs)-1].(*ssa.Panic)
	return panics
}
