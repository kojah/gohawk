package heapmodel

import (
	"container/list"
	"runtime"
	"slices"
	"sync"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Every cache of the heap model belongs to one SSA program: the summary
// registry, the graph cache, and the call-cycle inventory. buildssa builds a
// program per package, and every key is a function of that program, so an
// entry made for one package is never read while analyzing another. Keeping
// the caches per program lets them go when the package is done. Process-wide
// caches kept every analyzed package alive in a long-lived driver, such as
// the golangci-lint plugin, and their size limit made a package analyzed late
// in the process see fewer summaries than the same package analyzed early.

// programCaches is the state one SSA program owns.
type programCaches struct {
	summaries heapSummaryRegistry
	graphs    regionGraphCacheState
	cycles    callCycleCache
	// bound records that the caches are released with the program's
	// buildssa result.
	bound bool
}

var programs = struct {
	sync.Mutex
	caches map[*ssa.Program]*programCaches
}{caches: map[*ssa.Program]*programCaches{}}

// cachesOf returns the caches of the function's program, creating them on
// first use. A nil function, or one with no program, shares the caches of
// the nil program.
func cachesOf(function *ssa.Function) *programCaches {
	var program *ssa.Program
	if function != nil {
		program = function.Prog
	}
	programs.Lock()
	defer programs.Unlock()
	return programCachesLocked(program)
}

func programCachesLocked(program *ssa.Program) *programCaches {
	caches := programs.caches[program]
	if caches == nil {
		caches = &programCaches{
			summaries: heapSummaryRegistry{entries: map[*ssa.Function]heapEntry{}, generations: map[*ssa.Function]int{}},
			graphs: regionGraphCacheState{
				order: list.New(), entries: map[*ssa.Function]*list.Element{},
				dependents: map[*ssa.Function]map[*regionGraphEntry]bool{},
			},
			cycles: callCycleCache{reach: map[*ssa.Function]map[*ssa.Function]bool{}, metadata: map[*ssa.Function]*callCycleMetadata{}},
		}
		programs.caches[program] = caches
	}
	return caches
}

// BindProgramLifetime releases the caches of the result's program once the
// result itself is unreachable. A weak table keyed by the program could not
// do this: the cached graphs and summaries point back into the program, so
// it would never become unreachable. The buildssa result is a small value
// that only the analysis driver holds, and nothing here points at it.
//
// Every analyzer that reads the heap model lists buildssa among its
// requirements, so the result stays reachable for as long as any of them is
// analyzing the package. Binding is per program and idempotent; a program no
// one binds keeps its caches for the life of the process, which is harmless
// for go vet, where each package is analyzed in a process of its own.
func BindProgramLifetime(result *buildssa.SSA) {
	if result == nil || result.Pkg == nil || result.Pkg.Prog == nil {
		return
	}
	program := result.Pkg.Prog
	programs.Lock()
	caches := programCachesLocked(program)
	bound := caches.bound
	caches.bound = true
	programs.Unlock()
	if !bound {
		runtime.AddCleanup(result, releaseProgram, program)
	}
}

// BindPassProgram binds the lifetime of the pass's SSA program, when the
// pass has buildssa's result.
func BindPassProgram(pass *analysis.Pass) {
	if result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA); ok {
		BindProgramLifetime(result)
	}
}

func releaseProgram(program *ssa.Program) {
	programs.Lock()
	delete(programs.caches, program)
	programs.Unlock()
}

// The registry hands heap summaries to the graph. The lifecycle pass
// registers the summary of every callee it imports before it builds any
// graph of its own package, and its own summaries once they are computed,
// so a call to a summarized function is applied by substitution instead of
// forgetting every object the caller did not allocate.

// heapSummaryLimit bounds the registry. Past it nothing more is registered
// and a later lookup that misses is merely conservative. The registry never
// forgets what it holds: a summary that came and went with the order of
// requests would make the same package analyze differently from one run
// to the next.
const heapSummaryLimit = 131072

// heapEntryState is where a callee's summary stands in the registry. A
// callee being projected right now can be requested by another analyzer;
// that request projects privately instead of treating contention as recursion.
// Graph construction cuts call cycles before requesting a callee summary.
// An unavailable projection is remembered to avoid retrying at every replay.
type heapEntryState uint8

const (
	heapEntryReady heapEntryState = iota
	heapEntryProjecting
	heapEntryMissing
)

// An entry projected on demand came from the function's own graph, and is
// dropped when that graph is found stale; a registered one stands until it
// is registered again.
type heapEntry struct {
	summary  HeapSummary
	state    heapEntryState
	onDemand bool
}

// heapSummaryRegistry holds one program's summaries. generations counts, per
// function, the registrations that changed its summary and the on-demand
// summaries dropped. A graph remembers the generation of each summary it
// consulted; a projection finishing on demand, which only fills in what a
// cycle cut, does not count, so graphs around a call cycle are not rebuilt
// forever.
type heapSummaryRegistry struct {
	sync.Mutex
	entries     map[*ssa.Function]heapEntry
	generations map[*ssa.Function]int
}

// RegisterHeapSummary makes the summary available to every graph built
// afterwards for calls to the function. A summary that differs from what
// the registry held evicts the cached graphs that consulted the old one.
func RegisterHeapSummary(function *ssa.Function, summary HeapSummary) {
	if function == nil {
		return
	}
	registry := &cachesOf(function).summaries
	registry.Lock()
	previous, ok := registry.entries[function]
	if !ok && len(registry.entries) >= heapSummaryLimit {
		registry.Unlock()
		return
	}
	registry.entries[function] = heapEntry{summary: summary}
	changed := !ok || previous.state != heapEntryReady || !heapSummariesEqual(previous.summary, summary)
	if changed {
		registry.generations[function]++
	}
	registry.Unlock()
	if changed {
		invalidateDependents(function)
	}
}

// heapSummaryGeneration returns how many times the function's summary has
// been replaced or dropped.
func heapSummaryGeneration(function *ssa.Function) int {
	registry := &cachesOf(function).summaries
	registry.Lock()
	defer registry.Unlock()
	return registry.generations[function]
}

// forgetOnDemandSummary drops a summary projected from a graph that was
// found stale, and reports whether there was one to drop.
func forgetOnDemandSummary(function *ssa.Function) bool {
	registry := &cachesOf(function).summaries
	registry.Lock()
	defer registry.Unlock()
	entry, ok := registry.entries[function]
	if !ok || !entry.onDemand {
		return false
	}
	delete(registry.entries, function)
	registry.generations[function]++
	return true
}

// heapSummaryOf returns the callee's summary: the registered one, or, for a
// callee whose body is in the program, one projected on demand and kept.
// An instantiation of a generic function is answered from its own body
// whenever it has one, never from its origin: the origin's body is typed
// over type parameters and projects far less, and which of the two was
// registered first must not decide what a caller sees. The origin answers
// only for an instantiation the program did not build.
func heapSummaryOf(function *ssa.Function) (HeapSummary, bool) {
	if function == nil {
		return HeapSummary{}, false
	}
	if summary, ok := registeredHeapSummary(function); ok {
		return summary, true
	}
	if len(function.Blocks) != 0 {
		return projectHeapOnDemand(function)
	}
	resolved := ssacall.ResolvedFunction(function)
	if summary, ok := registeredHeapSummary(resolved); ok {
		return summary, true
	}
	return projectHeapOnDemand(resolved)
}

// RegisteredHeapSummary returns the registry's ready summary for the function.
// It never projects a summary and reports no answer while projection is in
// progress or when it was unavailable.
func RegisteredHeapSummary(function *ssa.Function) (HeapSummary, bool) {
	return registeredHeapSummary(function)
}

// registeredHeapSummary reports the registry's answer for one function:
// the summary when it is ready, and no answer while it is being projected
// or when its projection was found unavailable.
func registeredHeapSummary(function *ssa.Function) (HeapSummary, bool) {
	registry := &cachesOf(function).summaries
	registry.Lock()
	defer registry.Unlock()
	entry, ok := registry.entries[function]
	return entry.summary, ok && entry.state == heapEntryReady
}

// projectHeapOnDemand projects a callee whose body is in the program and
// keeps the result. A graph never applies a summary from its own call cycle
// (sameCallCycle), so a callee found mid-projection is being projected by
// another analyzer, not reached again on this stack. Treating that as a cut
// gave the caller no summary whenever two analyzers asked at once, and the
// same package was reported differently from run to run; the callee is
// projected privately instead, which yields the summary the other analyzer
// is about to register.
func projectHeapOnDemand(function *ssa.Function) (HeapSummary, bool) {
	if function == nil || len(function.Blocks) == 0 {
		return HeapSummary{}, false
	}
	registry := &cachesOf(function).summaries
	registry.Lock()
	if entry, ok := registry.entries[function]; ok {
		registry.Unlock()
		if entry.state == heapEntryProjecting {
			return ProjectHeap(function)
		}
		return entry.summary, entry.state == heapEntryReady
	}
	registry.entries[function] = heapEntry{state: heapEntryProjecting}
	registry.Unlock()
	summary, ok := ProjectHeap(function)
	registry.Lock()
	defer registry.Unlock()
	// The function may have been registered while this projection ran. The
	// registration stands: overwriting it with the weaker on-demand summary
	// made what later graphs saw depend on which write landed last.
	// https://github.com/kojah/gohawk/issues/69
	// An entry dropped meanwhile, by an eviction that forgot an older
	// on-demand summary, leaves this projection to store its own.
	if current, exists := registry.entries[function]; exists && current.state != heapEntryProjecting {
		return current.summary, current.state == heapEntryReady
	}
	if ok {
		registry.entries[function] = heapEntry{summary: summary, onDemand: true}
		return summary, true
	}
	registry.entries[function] = heapEntry{state: heapEntryMissing}
	return summary, false
}

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

// callCycleCache memoizes, per function, the functions of its package it
// can reach through static calls. A package's call graph cannot leave it
// and come back, because imports are acyclic, so the walk stays inside it.
type callCycleCache struct {
	sync.Mutex
	reach    map[*ssa.Function]map[*ssa.Function]bool
	metadata map[*ssa.Function]*callCycleMetadata
}

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
	callerOrigin, calleeOrigin := ssacall.ResolvedFunction(function), ssacall.ResolvedFunction(callee)
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
	cycles := &cachesOf(function).cycles
	cycles.Lock()
	reach, ok := cycles.reach[function]
	cycles.Unlock()
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
	cycles.Lock()
	cycles.reach[function] = reach
	cycles.Unlock()
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
	cycles := &cachesOf(function).cycles
	cycles.Lock()
	metadata := cycles.metadata[function]
	cycles.Unlock()
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
		if origin := ssacall.ResolvedFunction(callee); origin != callee {
			metadata.callees = append(metadata.callees, origin)
		}
	}
	cycles.Lock()
	defer cycles.Unlock()
	if published := cycles.metadata[function]; published != nil {
		return published
	}
	cycles.metadata[function] = metadata
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
