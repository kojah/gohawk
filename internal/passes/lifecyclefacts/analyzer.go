// Package lifecyclefacts resolves lifecycle evidence across source and package
// boundaries. It combines memoized local SSA proofs with conservative exported
// summaries, and keeps missing summaries distinct from disproved ownership.
package lifecyclefacts

import (
	"maps"
	"reflect"
	"slices"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	ssa "golang.org/x/tools/go/ssa"
)

// Analyzer is an internal prerequisite shared by lifecycle analyzers.
var Analyzer = &analysis.Analyzer{
	Name:       "lifecyclefacts",
	Doc:        "exports internal lifecycle ownership summaries",
	Requires:   []*analysis.Analyzer{buildssa.Analyzer},
	FactTypes:  []analysis.Fact{new(publishedFact), new(publishedCleanup), new(publishedPackage)},
	ResultType: reflect.TypeFor[Summaries](),
	Run:        run,
}

// traceAnalyzer names this prerequisite in a trace. It is not a catalog
// analyzer, but a reader selects its events the same way: -gohawk-trace=
// lifecyclefacts shows which function the fact pass is working on.
const traceAnalyzer = "lifecyclefacts"

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	summaries := make(Summaries, len(functions))
	callbacks := newCallbackInference(pass, summaries)
	marker := &SummarizedPackage{}
	// Import dependency summaries first, and hand their heap projections to
	// the graph, so every graph built for this package applies a summarized
	// callee by substitution instead of forgetting what the caller holds.
	// Facts belong to this prerequisite analyzer; siblings read them through
	// the result. A callee of this package has no fact yet and stays
	// unknown here; its own summary is registered once computed.
	importCalleeSummaries(pass, functions, summaries)
	var local []*ssa.Function
	// Callees before callers, so a function's projection is registered
	// before any caller in the package is summarized and the caller's graph
	// applies it. Every body's projection is registered, not only the
	// exported ones: a private helper that stores what it is handed is
	// exactly what a caller's graph must know about.
	for _, function := range calleesFirst(functions) {
		if object := function.Object(); object != nil && object.Exported() && len(function.Blocks) == 0 {
			marker.Bodiless = append(marker.Bodiless, object.Name())
		}
		object := function.Object()
		if len(function.Blocks) != 0 && (object == nil || !object.Exported() || len(function.Params) > 64) {
			heapmodel.RegisterHeapSummary(function, *projectHeap(function))
		}
		// Only exported functions can be called from a package that imports this
		// fact. Skipping private dependency helpers keeps the prerequisite linear
		// in the externally visible API instead of every transitive SSA body.
		if object == nil || !object.Exported() || len(function.Params) > 64 || len(function.Blocks) == 0 {
			continue
		}
		// Summarizing walks the callee graph, so a pathological package can
		// spend a long time on one function. Announce the function before the
		// walk as well as after it: a run that stops making progress is then
		// located by its last candidate rather than by a stack dump.
		probe := analysisTrace.For(pass, traceAnalyzer, "", function.Pos())
		if probe.Enabled() {
			probe.Candidate(analysisTrace.Step{
				Reason:   reasonSummarizingFunction.String(),
				Outcome:  analysisTrace.OutcomeObserved,
				Pos:      function.Pos(),
				Function: function.String(),
			})
		}
		fact := callbacks.summarize(function)
		summaries[function] = fact
		local = append(local, function)
		if fact.Heap != nil {
			heapmodel.RegisterHeapSummary(function, *fact.Heap)
		}
		if probe.Enabled() {
			details := fact.traceDetails()
			maps.Copy(details, heapTraceDetails(function, fact.Heap))
			probe.Decision(analysisTrace.Step{
				Reason:   reasonFunctionSummarized.String(),
				Outcome:  analysisTrace.OutcomeAccepted,
				Pos:      function.Pos(),
				Function: function.String(),
				Details:  details,
			})
		}
	}
	// A returned view is decided once every method of this package is
	// summarized, because the releasing method usually lives beside the
	// constructor. Export afterwards. An importer must be able to tell a
	// callee proven to do nothing from one that was never summarized, and
	// only the latter is unknown; the package marker carries that
	// distinction, so an empty summary need not be serialized for every
	// function of every dependency.
	slices.Sort(marker.Bodiless)
	pass.ExportPackageFact(&publishedPackage{factcodec.Wrap(*marker)})
	for _, function := range local {
		fact := summaries[function]
		fact.Must.ReturnedView = returnedViews(pass, function, fact, summaries)
		summaries[function] = fact
		if !fact.empty() {
			pass.ExportObjectFact(function.Object(), publish(fact))
		}
		if fact.Heap != nil {
			heapmodel.RegisterHeapSummary(function, *fact.Heap)
		}
	}
	// A type's contract needs its constructor and its methods, so it is joined
	// once both are summarized rather than while either is being proved.
	exportCleanupContracts(pass, summaries)
	return summaries, nil
}

// calleesFirst orders the functions so that every static callee within the
// set precedes its callers, cycles broken where they close; a function's
// projection is then available to every caller summarized after it.
func calleesFirst(functions []*ssa.Function) []*ssa.Function {
	members := make(map[*ssa.Function]bool, len(functions))
	for _, function := range functions {
		members[function] = true
	}
	var ordered []*ssa.Function
	state := map[*ssa.Function]int{}
	var visit func(function *ssa.Function)
	visit = func(function *ssa.Function) {
		if state[function] != 0 {
			return
		}
		state[function] = 1
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if callee := ssaflow.ResolvedCallee(ssaflow.InstructionCall(instruction)); callee != nil && members[callee] {
					visit(callee)
				}
			}
		}
		state[function] = 2
		ordered = append(ordered, function)
	}
	for _, function := range functions {
		visit(function)
	}
	return ordered
}

// importCalleeSummaries imports the fact of every static callee the package
// resolves and registers each callee's heap projection with the graph.
func importCalleeSummaries(pass *analysis.Pass, functions []*ssa.Function, summaries Summaries) {
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				common := ssaflow.InstructionCall(instruction)
				if common == nil || common.StaticCallee() == nil {
					continue
				}
				if _, seen := summaries[common.StaticCallee()]; seen {
					continue
				}
				if fact, ok := importFact(pass, instruction); ok {
					summaries[common.StaticCallee()] = fact
					if fact.Heap != nil {
						heapmodel.RegisterHeapSummary(common.StaticCallee(), *fact.Heap)
					}
					// A callee that returns an owned struct is only useful together
					// with the summaries of that struct's methods, which no sibling
					// analyzer can import itself.
					if fact.Must.OwnedFields != 0 {
						importResultMethods(pass, common.StaticCallee(), summaries)
					}
				}
			}
		}
	}
}

// factFor returns a memoized local or previously imported dependency summary.
func factFor(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	common := ssaflow.InstructionCall(instruction)
	if pass == nil || common == nil || common.StaticCallee() == nil {
		return Fact{}, false
	}
	if summaries, ok := pass.ResultOf[Analyzer].(Summaries); ok {
		if fact, found := summaries[common.StaticCallee()]; found {
			return fact, true
		}
	}
	return Fact{}, false
}
