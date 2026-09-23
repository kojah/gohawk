package resultfacts

import (
	"go/types"
	"reflect"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

const factVersion = 2

// Fact publishes independent result guarantees and the proven relations
// between a result and a parameter or another result. A relation is an
// implication that held on every return under its assumption; its absence
// is not the opposite implication.
type Fact struct {
	Version   int
	Results   []Guarantee
	Relations []Relation
}

// AFact marks the result component for go/analysis serialization.
func (*Fact) AFact() {}

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
		if object == nil || !object.Exported() || function.Signature.Results().Len() == 0 {
			continue
		}
		summary := engine.Function(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if summary.Available {
			pass.ExportObjectFact(object, &Fact{
				Version: factVersion, Results: slices.Clone(summary.results), Relations: slices.Clone(summary.relations),
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
