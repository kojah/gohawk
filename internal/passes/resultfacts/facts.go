package resultfacts

import (
	"go/types"
	"reflect"
	"slices"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

const factVersion = 4

// Fact publishes independent result guarantees, the result cases that hold
// under a condition, and the results proven to be a parameter. A case is an
// implication that held on every return under its condition; its absence is
// not the opposite implication.
type Fact struct {
	Version      int
	Results      []Guarantee
	Cases        []ResultCase
	Returned     []ReturnedParameter
	NeverReturns bool
}

// publishedFact hides the result schema from gob's per-stream descriptors.
type publishedFact struct{ publication }

// An unexported embedded alias also hides the envelope's descriptor from gob.
type publication = factcodec.Envelope[Fact]

// Analyzer computes result knowledge only when required before dependency
// analysis. It does not require lifecycle or concurrency inference.
var Analyzer = &analysis.Analyzer{
	Name: "gohawkresultfacts", Doc: "exports bounded unconditional result guarantees",
	Requires: []*analysis.Analyzer{buildssa.Analyzer}, FactTypes: []analysis.Fact{new(publishedFact)},
	ResultType: reflect.TypeFor[*Engine](), Run: run,
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	engine := NewEngine()
	engine.imported = make(map[*types.Func]Fact)
	for _, imported := range pass.AllObjectFacts() {
		object, ok := imported.Object.(*types.Func)
		published, valid := imported.Fact.(*publishedFact)
		if ok && valid {
			fact := published.Value()
			if validFact(&fact) {
				engine.imported[object] = fact
			}
		}
	}
	for _, function := range functions {
		object := function.Object()
		// A function with no results still has something to say when it
		// never returns; every other claim needs a result to be about.
		if object == nil || !object.Exported() {
			continue
		}
		summary := engine.Function(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if summary.Available {
			pass.ExportObjectFact(object, &publishedFact{factcodec.Wrap(Fact{
				Version: factVersion, Results: slices.Clone(summary.results), Cases: slices.Clone(summary.cases), Returned: slices.Clone(summary.returned),
				NeverReturns: summary.neverReturns,
			})})
		}
	}
	return engine, nil
}

func validFact(fact *Fact) bool {
	if fact.Version != factVersion || len(fact.Results) > maxResults {
		return false
	}
	for _, result := range fact.Results {
		if result > AlwaysFalse {
			return false
		}
	}
	for _, proven := range fact.Cases {
		if proven.Result < 0 || proven.Result >= len(fact.Results) || proven.Outcome == ssaflow.OutcomeAny || proven.Outcome > ssaflow.OutcomeNonNil {
			return false
		}
	}
	for _, returned := range fact.Returned {
		if returned.Result < 0 || returned.Result >= len(fact.Results) || returned.Parameter < 0 || returned.Parameter >= 64 {
			return false
		}
	}
	return true
}
