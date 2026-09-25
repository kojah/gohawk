package ssaflow

import "golang.org/x/tools/go/ssa"

// RunsOnceInProgramEntry reports whether instruction executes at most once
// per process because it sits in the program's entry function outside any
// loop. The Go specification makes main.main of package main the entry, and
// the program exits when it returns, so this is a language contract rather
// than a naming guess: a function called main in any other package, a method,
// or a closure declared inside main does not qualify, because each can run
// more than once. A package that calls or refers to its own main could run it
// again, so any such reference also declines.
//
// This is only the "at most once, until exit" half of a process-lifetime
// argument. Whether exit actually settles an obligation, rather than losing a
// flush or a commit, is the calling analyzer's decision.
func RunsOnceInProgramEntry(instruction ssa.Instruction) bool {
	function := instruction.Parent()
	if !programEntry(function) || BlockInCycle(instruction.Block()) {
		return false
	}
	for _, other := range DeclaredFunctions(function.Pkg) {
		if refersTo(other, function) {
			return false
		}
	}
	return true
}

func programEntry(function *ssa.Function) bool {
	return function != nil && function.Pkg != nil && function.Pkg.Pkg.Name() == "main" &&
		function.Name() == "main" && function.Parent() == nil && function.Signature.Recv() == nil &&
		function.Object() != nil && function.Pkg.Pkg.Scope().Lookup("main") == function.Object()
}

func refersTo(function, target *ssa.Function) bool {
	var operands []*ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			operands = instruction.Operands(operands[:0])
			for _, operand := range operands {
				if operand != nil && *operand == target {
					return true
				}
			}
		}
	}
	return false
}
