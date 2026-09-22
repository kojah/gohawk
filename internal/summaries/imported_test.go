package summaries

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestBrokerLocalAndImportedParity(t *testing.T) {
	selection := Select(Requirements{Results: true, Lifecycle: true, Concurrency: true})
	consumer := &analysis.Analyzer{
		Name: "broker", Doc: "assert modular summary access", Requires: selection.Requires(),
		Run: func(pass *analysis.Pass) (any, error) {
			provider := selection.Provider(pass)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			checked := 0
			for _, function := range functions {
				if function.Name() != "Close" && function.Name() != "Lock" && function.Name() != "Result" {
					continue
				}
				call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
				for _, target := range []*ssa.Function{function, call.Common().StaticCallee()} {
					assertBrokerDeclaration(t, provider.ForFunction(target), function.Name())
				}
				if function.Name() == "Lock" {
					bound, available := provider.ConcurrencyAtCall(call, ssaflow.NewSearchBudget(2000))
					if available != Available || !bound.Complete() || len(bound.Operations) != 2 {
						t.Fatalf("bound effects: %+v", bound)
					}
					for _, operation := range bound.Operations {
						if operation.Resource.Value != function.Params[0] {
							t.Error("formal effect was not bound to caller")
						}
					}
				}
				checked++
			}
			if checked != 3 {
				t.Fatalf("checked %d declarations", checked)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "brokerconsumer")
}

func assertBrokerDeclaration(t *testing.T, view Function, name string) {
	t.Helper()
	switch name {
	case "Close":
		fact, available := view.Lifecycle()
		if available != Available || fact.Closed != 1 {
			t.Fatalf("lifecycle declaration: %+v (%v)", fact, available)
		}
	case "Lock":
		fact, available := view.Concurrency(ssaflow.NewSearchBudget(2000))
		if available != Available || len(fact.Effects) != 2 {
			t.Fatalf("concurrency declaration: %+v (%v)", fact, available)
		}
		for _, effect := range fact.Effects {
			if effect.Parameter != 0 {
				t.Error("declaration lost formal parameter")
			}
		}
	case "Result":
		fact, available := view.Results(ssaflow.NewSearchBudget(2000))
		if available != Available || fact.Result(0) != resultfacts.AlwaysNil {
			t.Fatalf("result declaration: %+v (%v)", fact, available)
		}
	}
}
