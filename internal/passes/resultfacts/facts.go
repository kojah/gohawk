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

const factVersion = 3

// Fact publishes independent result guarantees and the proven relations
// between a result and a parameter or another result. A relation is an
// implication that held on every return under its assumption; its absence
// is not the opposite implication.
type Fact struct {
	Version      int
	Results      []Guarantee
	Relations    []Relation
	NeverReturns bool
}

// AFact marks the result component for go/analysis serialization.
func (*Fact) AFact() {}

// GobEncode encodes the fact through factcodec.
func (fact *Fact) GobEncode() ([]byte, error) { return factcodec.Encode(fact) }

// GobDecode decodes the fact through factcodec.
func (fact *Fact) GobDecode(data []byte) error { return factcodec.Decode(data, fact) }

// Analyzer computes result knowledge only when required before dependency
// analysis. It does not require lifecycle or concurrency inference.
var Analyzer = &analysis.Analyzer{
	Name: "gohawkresultfacts", Doc: "exports bounded unconditional result guarantees",
	Requires: []*analysis.Analyzer{buildssa.Analyzer}, FactTypes: []analysis.Fact{new(Fact)},
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
		fact, valid := imported.Fact.(*Fact)
		if ok && valid && validFact(fact) {
			engine.imported[object] = *fact
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
			pass.ExportObjectFact(object, &Fact{
				Version: factVersion, Results: slices.Clone(summary.results), Relations: slices.Clone(summary.relations),
				NeverReturns: summary.neverReturns,
			})
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
	for _, relation := range fact.Relations {
		if relation.Kind == 0 || relation.Kind > ReturnsParameter || relation.Result < 0 ||
			relation.Result >= len(fact.Results) || relation.Operand < 0 || relation.Operand > maxResults {
			return false
		}
	}
	return true
}
