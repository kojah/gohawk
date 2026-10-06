package ssaflow

import (
	"go/types"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

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
	// A package-level alias stores main in the synthetic initializer, which
	// is not a declared source function. It can invoke the entry again too.
	if initializer := function.Pkg.Func("init"); initializer != nil && refersTo(initializer, function) {
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

// RunsOnceThroughPrivateEntryCallsWithin extends the at-most-once entry witness
// through a chain of private nonescaping declarations with one synchronous
// caller each. Every call and the instruction must be outside CFG cycles. The
// complete package use census rejects callbacks, aliases, repeated calls and
// references to main. Generic declarations and chains beyond 16 frames decline.
// The supplied allowance owns body/operand and CFG searches; package metadata
// enumeration retains DeclaredFunctions' existing independent cost. This says
// nothing about cleanup or whether entry eventually returns.
func RunsOnceThroughPrivateEntryCallsWithin(instruction ssa.Instruction, budget *proofs.SearchBudget) bool {
	function := instruction.Parent()
	if function == nil || function.Pkg == nil || function.Pkg.Pkg.Name() != "main" {
		return false
	}
	pkg := function.Pkg
	functions := append([]*ssa.Function{pkg.Func("init")}, DeclaredFunctions(pkg)...)
	uses := CollectPrivateFunctionUsesWithin(functions, budget)
	if uses == nil {
		return false
	}
	for range 16 {
		if !budget.Spend() {
			return false
		}
		function = instruction.Parent()
		if function == nil || function.Pkg != pkg || BlockInCycleWithin(instruction.Block(), budget) || budget.Exhausted() {
			return false
		}
		entry := uses[function]
		if programEntry(function) {
			return !entry.Escaped && len(entry.Calls) == 0
		}
		if !privateEntryDeclaration(function) || entry.Escaped || len(entry.Calls) != 1 {
			return false
		}
		instruction = entry.Calls[0]
	}
	return false
}

// The package declaration, rather than an instantiated signature, decides
// privacy and generic status. Instances can erase their signature parameters.
func privateEntryDeclaration(function *ssa.Function) bool {
	object := function.Object()
	if object == nil || object.Exported() || function.Parent() != nil || function.Signature.Recv() != nil {
		return false
	}
	return function.Signature.TypeParams().Len() == 0 && object.Type().(*types.Signature).TypeParams().Len() == 0 &&
		function.Pkg.Pkg.Scope().Lookup(object.Name()) == object
}
