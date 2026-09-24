package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func TestImportedEffects(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testeffects", Doc: "assert imported ordered effects", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			checked := 0
			for _, function := range functions {
				switch function.Name() {
				case "forward", "reverse":
					result := engine.Function(function, ssaflow.NewSearchBudget(2000))
					if result.Completeness() != CompleteWithEffects || len(result.Operations) != 4 {
						t.Fatalf("%s: %+v", function, result)
					}
					indices := []int{0, 1, 1, 0}
					if function.Name() == "reverse" {
						indices = []int{1, 0, 0, 1}
					}
					for i, kind := range []Kind{Lock, Lock, Unlock, Unlock} {
						if op := result.Operations[i]; op.Kind != kind || op.Resource.Value != function.Params[indices[i]] {
							t.Errorf("%s event %d: %+v", function, i, op)
						}
					}
					checked++
				case "spawning":
					assertImportedWorker(t, function, engine.Function(function, ssaflow.NewSearchBudget(2000)))
					checked++
				case "opaque", "conditional", "localOnly":
					result := engine.Function(function, ssaflow.NewSearchBudget(2000))
					if result.Completeness() != Incomplete || result.Complete() || result.Reason == ReasonNone {
						t.Errorf("%s unexpectedly complete: %+v", function, result)
					}
					checked++
				case "empty":
					result := engine.Function(function, ssaflow.NewSearchBudget(2000))
					if result.Completeness() != CompleteNoEffects || !result.Complete() || len(result.Operations) != 0 {
						t.Errorf("complete empty effects lost: %+v", result)
					}
					checked++
				}
			}
			if checked != 7 {
				t.Errorf("checked %d functions, want 7", checked)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "consumer")
}

func assertImportedWorker(t *testing.T, function *ssa.Function, result Summary) {
	t.Helper()
	if result.Completeness() != CompleteWithEffects || len(result.Workers) != 1 ||
		len(result.Workers[0].Operations) != 2 {
		t.Errorf("imported child launch = %+v", result)
		return
	}
	for i, kind := range []Kind{Lock, Unlock} {
		if op := result.Workers[0].Operations[i]; op.Kind != kind || op.Resource.Value != function.Params[0] {
			t.Errorf("imported child operation %d = %+v", i, op)
		}
	}
}
