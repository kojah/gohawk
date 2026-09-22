package resultfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func TestCrossPackageResults(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "resultconsumer", Doc: "assert cross-package result composition",
		Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			want := map[string][]Guarantee{
				"Nil": {AlwaysNil}, "TypedNil": {AlwaysNonNil}, "Pair": {AlwaysFalse, AlwaysNil},
				"Mixed": {Unknown}, "Global": {Unknown}, "Deferred": {Unknown},
			}
			for _, function := range functions {
				expected, ok := want[function.Name()]
				if !ok {
					continue
				}
				local := engine.Function(function, ssaflow.NewSearchBudget(2000))
				call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
				imported := engine.Function(call.Common().StaticCallee(), ssaflow.NewSearchBudget(2000))
				if !local.Available || !imported.Available {
					t.Errorf("%s: missing component", function)
				}
				for index, guarantee := range expected {
					if local.Result(index) != guarantee || imported.Result(index) != guarantee {
						t.Errorf("%s result %d: local=%v imported=%v want=%v", function, index, local.Result(index), imported.Result(index), guarantee)
					}
				}
				delete(want, function.Name())
			}
			if len(want) != 0 {
				t.Errorf("missing cases: %v", want)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "resultconsumer")
}
