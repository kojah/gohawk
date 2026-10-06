package concurrencyfacts

import (
	"go/types"
	"sync"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// The engine owns package caches and serializes public evidence queries. Rich
// local queries and linear fact publication keep separate cache policies;
// incomplete collection retains the cutoff that explains its uncertainty.

// Engine caches evidence for one package. Concurrent consumers are serialized
// at the public query boundary; each query must supply its own work budget.
type Engine struct {
	cutoff    *summaryCutoff
	mu        sync.Mutex
	summaries *ssacall.FunctionSummaries[Summary]
	linear    *ssacall.FunctionSummaries[Summary]
	paths     bool
	budget    *proofs.SearchBudget
	storage   *heapmodel.Storage
	facts     map[*types.Func]Fact
	// fields is shared by every query of the engine; see identity.go.
	fields *fieldInventory
}

// NewEngine builds a local-only engine; Analyzer additionally loads dependency facts.
func NewEngine() *Engine {
	engine := &Engine{fields: &fieldInventory{canonical: map[*ssa.UnOp]*ssa.UnOp{}}}
	engine.summaries = engine.newSummaries(true)
	engine.linear = engine.newSummaries(false)
	return engine
}

// Fact publication cannot represent alternatives. Give it a separate, fixed
// cache policy so exported functions do not pay for paths only graph queries
// can use. A linear cutoff must not poison the richer query's cache.
func (engine *Engine) newSummaries(paths bool) *ssacall.FunctionSummaries[Summary] {
	return ssacall.NewFunctionSummaries(func(function *ssa.Function, budget *proofs.SearchBudget) Summary {
		builder := engine.query(budget)
		builder.paths = paths
		if !paths {
			builder.summaries = engine.linear
		}
		return builder.collect(function, false)
	}, unavailableSummary)
}

func unavailableSummary(reason ssacall.SummaryUnavailable) Summary {
	switch reason {
	case ssacall.SummaryRecursive:
		return Summary{Reason: ReasonRecursiveProtocol}
	case ssacall.SummaryBudgetExhausted:
		return Summary{Reason: ReasonBudgetExhausted}
	case ssacall.SummaryBodyUnavailable:
		return Summary{Reason: ReasonBodyUnavailable}
	}
	return Summary{Reason: ReasonEffectUnknown}
}

func (engine *Engine) query(budget *proofs.SearchBudget) *Engine {
	return &Engine{
		summaries: engine.summaries, facts: engine.facts, budget: budget, storage: heapmodel.NewStorage(budget), paths: true,
		fields: engine.fields,
	}
}

// Function summarizes a visible body, including bounded child templates.
func (engine *Engine) Function(function *ssa.Function, budget *proofs.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.summaries.Function(function, budget)
}

// Root collects a caller and at most maxWorkers children under one shared work budget.
//
// A root summary answers what can happen before the root returns, for
// consumers that prove a wait which blocks before any return. Unlike a
// composed helper summary, it treats values the root returns as reaching the
// caller only after the root returns, so a returned reference is not a new
// participant. A consumer that reasons about effects after the root returns
// must not use Root.
//
// Every summary, root or helper, admits instructions that can panic on a nil
// owner or bad index. A function that recovers is never complete, so a panic
// ends its path before any later event.
func (engine *Engine) Root(function *ssa.Function, budget *proofs.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	query := engine.query(budget)
	return query.materialize(query.collect(function, true), function)
}

// AtCall binds complete local or imported effects to the caller's exact values.
func (engine *Engine) AtCall(call ssa.CallInstruction, budget *proofs.SearchBudget) Summary {
	// Analysis drivers may run sibling consumers in parallel. The cache and
	// its recursion guard form one transaction, so locking individual map
	// accesses would still let another query look like a recursive call.
	engine.mu.Lock()
	defer engine.mu.Unlock()
	query := engine.query(budget)
	return query.materialize(query.callSummary(call), call.Parent())
}

func (engine *Engine) collect(function *ssa.Function, root bool) Summary {
	engine.cutoff = nil
	result := engine.collectEffects(function, root)
	if engine.paths && result.Reason == ReasonControlFlowUnknown && function != nil && len(function.Blocks) != 0 {
		// The acyclic pass located the loop header. Keep that explanation if
		// the counted-loop retry fails too; the retry only knows the first
		// branch it could not expand.
		located := engine.cutoff
		engine.cutoff = nil
		result = engine.collectCountedLoops(function, root)
		if result.Reason == ReasonControlFlowUnknown && located != nil {
			engine.cutoff = located
		}
	}
	if engine.paths && (result.Reason == ReasonBranchEffectsDiffer || result.Reason == ReasonBranchAlternatives) {
		engine.cutoff = nil
		result = engine.collectPaths(function, root)
	}
	result = finishCancellation(result)
	if result.Reason == ReasonCallbackBindingRequired && engine.cutoff == nil {
		engine.recordCutoff(holeInstruction(function, result), cutoffInstruction)
	}
	if result.Reason != ReasonNone && !result.AlternativesComplete && len(result.Paths) == 0 {
		result.cutoff = engine.cutoff
		if result.cutoff == nil {
			result.cutoff = &summaryCutoff{function: function}
		}
	}
	return result
}
