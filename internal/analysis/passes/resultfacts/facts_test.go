package resultfacts

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
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
				"StoredTrue": {AlwaysTrue}, "StoredTypedNil": {AlwaysNonNil},
			}
			for _, function := range functions {
				expected, ok := want[function.Name()]
				if !ok {
					continue
				}
				local := engine.Function(function, proofs.NewSearchBudget(2000))
				call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
				imported := engine.Function(call.Common().StaticCallee(), proofs.NewSearchBudget(2000))
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

// The pointer wrapper needs the imported heap summary; the slice wrapper
// shares storage with its argument but does not return the original slice.
func TestImportedReturnedIdentity(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "identityconsumer", Doc: "assert identity after imported heap publication",
		Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			checked := 0
			for _, function := range functions {
				want := function.Name() == "Pointer"
				if !want && function.Name() != "SliceView" && function.Name() != "TupleView" {
					continue
				}
				call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
				for _, target := range []*ssa.Function{function, call.Common().StaticCallee()} {
					summary := engine.Function(target, proofs.NewSearchBudget(proofs.SummaryBudget))
					result := 0
					if function.Name() == "TupleView" {
						result = 1
					}
					parameter, proven := summary.ReturnedParameter(result)
					if !summary.Available || proven != want || proven && parameter != 0 {
						t.Errorf("%s: returned parameter %d, proven %t, available %t; want proven %t", target, parameter, proven, summary.Available, want)
					}
				}
				checked++
			}
			if checked != 3 {
				t.Fatalf("checked %d wrappers, want 3", checked)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "resultconsumer")
}
