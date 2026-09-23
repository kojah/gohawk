// Package resultfacts proves bounded unconditional guarantees about individual
// function results. Guarantees concern every normal return, not ownership,
// termination, or relationships between different result positions.
package resultfacts

import (
	"go/constant"
	"go/types"
	"sync"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Guarantee describes one result on every normal return. Unknown includes
// conflicting evidence, unsupported values, and absence of a return witness.
type Guarantee uint8

const (
	Unknown Guarantee = iota
	AlwaysNil
	AlwaysNonNil
	AlwaysTrue
	AlwaysFalse
)

const maxResults = 16

// Summary is immutable after publication. Available means inference could be
// consulted, not that all results are understood. Reason explains a boundary.
type Summary struct {
	Available bool
	Reason    string
	results   []Guarantee
	relations []Relation
	// neverReturns records that no normal return is reachable from the
	// entry: every path ends in a terminating call, a panic, or a loop that
	// never exits. See NeverReturns.
	neverReturns bool
}

// NeverReturns reports whether the function is proven never to return
// normally, so a call to it terminates the caller's path as os.Exit does.
// It is a claim about every path, proven from the body or imported; a
// function that merely may exit does not carry it.
func (summary Summary) NeverReturns() bool {
	return summary.Available && summary.neverReturns
}

// Result returns the unconditional guarantee at index, or Unknown.
func (summary Summary) Result(index int) Guarantee {
	if index < 0 || index >= len(summary.results) {
		return Unknown
	}
	return summary.results[index]
}

// Engine shares result inference for a package. Public queries serialize cache
// access; recursion and budget handling are owned by FunctionSummaries.
type Engine struct {
	mu        sync.Mutex
	summaries *ssaflow.FunctionSummaries[Summary]
	imported  map[*types.Func]Fact
}

// NewEngine creates local-only result inference with no library-name guesses.
func NewEngine() *Engine {
	engine := &Engine{}
	engine.summaries = ssaflow.NewFunctionSummaries(engine.compute, func(reason ssaflow.SummaryUnavailable) Summary {
		return Summary{Reason: "result-summary-unavailable"}
	})
	return engine
}

// Function returns local or imported, context-independent result guarantees.
func (engine *Engine) Function(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.function(function, budget)
}

func (engine *Engine) function(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
	if function == nil || !budget.Spend() {
		return Summary{Reason: "result-summary-unavailable"}
	}
	if len(function.Blocks) == 0 {
		object, _ := function.Object().(*types.Func)
		if fact, ok := engine.imported[object]; ok && fact.Version == factVersion {
			return Summary{Available: true, results: fact.Results, relations: fact.Relations, neverReturns: fact.NeverReturns}
		}
		return Summary{Reason: "result-body-unavailable"}
	}
	return engine.summaries.Function(function, budget)
}

func (engine *Engine) compute(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
	count := function.Signature.Results().Len()
	if count > maxResults {
		return Summary{Reason: "result-count-limit"}
	}
	result := Summary{Available: true, results: make([]Guarantee, count)}
	witness := false
	// Include recovery returns too. Ignoring the detached recovery block could
	// claim a literal result even when a recovering defer returns a zero value.
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return Summary{Reason: "result-budget-exhausted"}
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			for index, value := range returned.Results {
				guarantee := engine.value(value, budget)
				if !witness {
					result.results[index] = guarantee
				} else if result.results[index] != guarantee {
					result.results[index] = Unknown
				}
			}
			witness = true
		}
	}
	// A terminating callee is one the catalog names or one whose own summary
	// says it never returns, so the claim composes through a project's
	// fatal wrapper and across packages. A body with a recover block can
	// return normally from a panic the entry never reaches, so it makes no
	// claim.
	result.neverReturns = function.Recover == nil && !ssaflow.NormalReturnReachableWith(function.Blocks[0], func(call *ssa.Call) bool {
		return budget.Spend() && engine.function(ssaflow.ResolvedCallee(call.Common()), budget).NeverReturns()
	})
	if !witness {
		result.Reason = "result-no-normal-return-witness"
		return result
	}
	result.relations = engine.relations(function, budget)
	return result
}

func (engine *Engine) value(value ssa.Value, budget *ssaflow.SearchBudget) Guarantee {
	result, ok := ssaflow.ResolveReachingValue(
		ssaflow.NewReachingWalk(ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType), value,
		func(_ ssaflow.ReachingWalk, leaf ssa.Value) (Guarantee, bool) {
			guarantee := engine.leaf(leaf, budget)
			// Stop the reaching fold at uncertainty, especially a budget cut;
			// agreeing Unknown leaves must not keep expanding sibling phis.
			return guarantee, guarantee != Unknown
		},
		func(guarantee Guarantee) Guarantee { return guarantee },
	)
	if !ok || budget.Exhausted() {
		return Unknown
	}
	return result
}

func (engine *Engine) leaf(value ssa.Value, budget *ssaflow.SearchBudget) Guarantee {
	if !budget.Spend() {
		return Unknown
	}
	if call, index, ok := ssaflow.CallResultSource(value); ok {
		return engine.function(ssaflow.ResolvedCallee(call.Common()), budget).Result(index)
	}
	switch value := value.(type) {
	case *ssa.Const:
		if value.IsNil() {
			return AlwaysNil
		}
		if value.Value != nil && value.Value.Kind() == constant.Bool {
			if constant.BoolVal(value.Value) {
				return AlwaysTrue
			}
			return AlwaysFalse
		}
	case *ssa.MakeInterface:
		// Boxing a typed nil pointer still produces a nonnil interface.
		// A type parameter can itself instantiate to an interface, however;
		// its unknown dynamic type must not become a concrete boxing proof.
		if _, abstract := value.X.Type().Underlying().(*types.Interface); abstract {
			return Unknown
		}
		return AlwaysNonNil
	case *ssa.Alloc, *ssa.MakeChan, *ssa.MakeMap, *ssa.MakeSlice, *ssa.Function, *ssa.MakeClosure:
		return AlwaysNonNil
	}
	// Loads remain opaque, including package sentinels and named results that
	// a deferred callback may modify. An initializer is not a lifetime guarantee.
	return Unknown
}
