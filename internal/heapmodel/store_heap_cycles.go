package heapmodel

import "github.com/kojah/gohawk/internal/ssaflow"

import (
	"sync"

	"golang.org/x/tools/go/ssa"
)

// A function's graph never applies the summary of a callee that can call
// back into it. Such a summary depends on the function's own, so the one the
// registry holds when the graph is built is whatever approximation some
// earlier request left there: a cycle cut where another analyzer entered
// the cycle, or a placeholder projected while the graph was unavailable.
// go/ast.Walk's summary came out with 119 effects in one run and none in
// the next, and every caller of go/ast.Inspect inherited the difference.
// A call into the caller's own call cycle is instead cut the way a callee
// without a summary is, so each summary depends only on the callees outside
// its cycle and every run computes the same one.

// callCycleReach memoizes, per function, the functions of its package it
// can reach through static calls. A package's call graph cannot leave it
// and come back, because imports are acyclic, so the walk stays inside it.
var callCycleReach = struct {
	sync.Mutex
	reach map[*ssa.Function]map[*ssa.Function]bool
}{reach: map[*ssa.Function]map[*ssa.Function]bool{}}

// sameCallCycle reports whether the callee, called from function, can call
// back into function.
func sameCallCycle(function, callee *ssa.Function) bool {
	if function == nil || callee == nil {
		return false
	}
	if callee == function || ssaflow.ResolvedFunction(callee) == ssaflow.ResolvedFunction(function) {
		return true
	}
	return reachableCallees(callee)[function] || reachableCallees(ssaflow.ResolvedFunction(callee))[ssaflow.ResolvedFunction(function)]
}

// reachableCallees returns the functions of the same package the function
// reaches through static calls, itself excluded unless it recurses.
func reachableCallees(function *ssa.Function) map[*ssa.Function]bool {
	if function == nil {
		return nil
	}
	callCycleReach.Lock()
	reach, ok := callCycleReach.reach[function]
	callCycleReach.Unlock()
	if ok {
		return reach
	}
	home := functionPackage(function)
	reach = map[*ssa.Function]bool{}
	pending := staticCallees(function)
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if reach[next] || home == nil || functionPackage(next) != home {
			continue
		}
		reach[next] = true
		pending = append(pending, staticCallees(next)...)
	}
	callCycleReach.Lock()
	callCycleReach.reach[function] = reach
	callCycleReach.Unlock()
	return reach
}

// staticCallees lists the functions a body calls directly, each with the
// origin an instantiation resolves to.
func staticCallees(function *ssa.Function) []*ssa.Function {
	var callees []*ssa.Function
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if common == nil || common.StaticCallee() == nil {
				continue
			}
			callees = append(callees, common.StaticCallee())
			if resolved := ssaflow.ResolvedFunction(common.StaticCallee()); resolved != common.StaticCallee() {
				callees = append(callees, resolved)
			}
		}
	}
	return callees
}

// functionPackage names the package that owns a function's body; an
// instantiation belongs to its origin's package.
func functionPackage(function *ssa.Function) *ssa.Package {
	if function.Pkg != nil {
		return function.Pkg
	}
	if origin := function.Origin(); origin != nil {
		return origin.Pkg
	}
	return nil
}
