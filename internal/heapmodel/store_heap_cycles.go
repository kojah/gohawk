package heapmodel

import (
	"slices"
	"sync"

	"github.com/kojah/gohawk/internal/ssaflow"
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
	reach    map[*ssa.Function]map[*ssa.Function]bool
	metadata map[*ssa.Function]*callCycleMetadata
}{reach: map[*ssa.Function]map[*ssa.Function]bool{}, metadata: map[*ssa.Function]*callCycleMetadata{}}

// sameCallCycle reports whether the callee, called from function, can call
// back into function.
func sameCallCycle(function, callee *ssa.Function) bool {
	if function == nil || callee == nil {
		return false
	}
	// An instantiation wrapper only converts its arguments and calls its
	// generic origin, which never calls back into the wrapper. Calling the
	// origin from it is not recursion, and cutting it there left the
	// wrapper's result a fresh object, so Must(os.Create(path)) lost the
	// file it returns. A generic body that calls an instantiation of itself
	// is still a cycle, below.
	// https://github.com/koki-develop/gat/blob/f4ad44169fcf08177edf1eb604eab966b4b61b0a/docs/update.go#L56-L58
	if instantiationWrapperCallsOrigin(function, callee) {
		return false
	}
	callerOrigin, calleeOrigin := ssaflow.ResolvedFunction(function), ssaflow.ResolvedFunction(callee)
	if callee == function || calleeOrigin == callerOrigin {
		return true
	}
	return reachableCallees(callee)[function] || reachableCallees(calleeOrigin)[callerOrigin]
}

func instantiationWrapperCallsOrigin(function, callee *ssa.Function) bool {
	return function.Synthetic != "" && function.Origin() != nil && callee == function.Origin()
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
	metadata := cycleMetadata(function)
	home := metadata.home
	reach = map[*ssa.Function]bool{}
	// The queue grows into its popped tail. It must own that storage rather
	// than overwrite a callee inventory another root or analyzer reuses.
	pending := slices.Clone(metadata.callees)
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if reach[next] || home == nil || functionPackage(next) != home {
			continue
		}
		reach[next] = true
		pending = append(pending, cycleMetadata(next).callees...)
	}
	callCycleReach.Lock()
	callCycleReach.reach[function] = reach
	callCycleReach.Unlock()
	return reach
}

// callCycleMetadata is structural information about one built SSA body. Unlike
// heap summaries, these edges and package ownership do not change when a callee
// summary is registered. The published inventory is read-only.
type callCycleMetadata struct {
	home    *ssa.Package
	callees []*ssa.Function
}

// cycleMetadata shares body discovery across reachability roots. Competing
// readers may discover privately, but all reuse the first published inventory.
func cycleMetadata(function *ssa.Function) *callCycleMetadata {
	callCycleReach.Lock()
	metadata := callCycleReach.metadata[function]
	callCycleReach.Unlock()
	if metadata != nil {
		return metadata
	}
	metadata = &callCycleMetadata{home: functionPackage(function)}
	for instruction := range ssaflow.InstructionsWithin(function, nil) {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			continue
		}
		callee := common.StaticCallee()
		if callee == nil {
			continue
		}
		metadata.callees = append(metadata.callees, callee)
		if origin := ssaflow.ResolvedFunction(callee); origin != callee {
			metadata.callees = append(metadata.callees, origin)
		}
	}
	callCycleReach.Lock()
	defer callCycleReach.Unlock()
	if published := callCycleReach.metadata[function]; published != nil {
		return published
	}
	callCycleReach.metadata[function] = metadata
	return metadata
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
