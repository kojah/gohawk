package resultfacts

import (
	"go/types"
	"reflect"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

const factVersion = 1

// Fact publishes independent result guarantees, never conditional relations.
type Fact struct {
	Version int
	Results []Guarantee
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
		summary := engine.Function(function, ssaflow.NewSearchBudget(2000))
		if summary.Available {
			pass.ExportObjectFact(object, &Fact{Version: factVersion, Results: slices.Clone(summary.results)})
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
	return true
}
