package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

var alternativeChecks = map[string]func(*testing.T, *ssa.Function, Summary){
	"pickTwice": func(t *testing.T, function *ssa.Function, got Summary) {
		t.Helper()
		if len(got.Paths) != 4 {
			t.Fatalf("pickTwice = %+v, want four paths", got)
		}
		for _, path := range got.Paths {
			if len(path.Conditions) != 2 || path.Conditions[0].Value != function.Params[2] ||
				path.Conditions[1].Value != function.Params[2] {
				t.Errorf("pickTwice conditions = %+v, want both on the caller's x", path.Conditions)
			}
		}
	},
	"modeConstant": func(t *testing.T, _ *ssa.Function, got Summary) {
		t.Helper()
		if len(got.Paths) != 2 {
			t.Fatalf("modeConstant = %+v, want two paths", got)
		}
		for _, path := range got.Paths {
			if len(path.Conditions) != 1 || path.Conditions[0].Compared == nil {
				t.Errorf("modeConstant conditions = %+v, want one comparison", path.Conditions)
			}
		}
	},
	"useAcquire": func(t *testing.T, _ *ssa.Function, got Summary) {
		t.Helper()
		implied := 0
		for _, path := range got.Paths {
			for _, condition := range path.Conditions {
				if condition.Implied {
					implied++
				}
			}
		}
		if len(got.Paths) == 0 || implied == 0 {
			t.Errorf("useAcquire = %+v, want paths with implied result conditions", got)
		}
	},
}

// Published alternatives bind at the importing call: conditions on a formal
// become conditions on the caller's argument, a constant argument stays
// comparable with the published constant, and a return fact becomes an
// implied condition on the caller's result.
func TestImportedAlternatives(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testalternatives", Doc: "assert imported path alternatives", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			checked := 0
			for _, function := range functions {
				if check, ok := alternativeChecks[function.Name()]; ok {
					checked++
					check(t, function, engine.Function(function, ssaflow.NewSearchBudget(4000)))
				}
			}
			if checked != 3 {
				t.Errorf("checked %d functions, want 3", checked)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "branchuse")
}
