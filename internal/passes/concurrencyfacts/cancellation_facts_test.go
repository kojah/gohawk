package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

func TestImportedCancellationRequirements(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testcancellation", Doc: "assert conditional cancellation binding", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			for _, function := range functions {
				result := engine.Root(function, ssaflow.NewSearchBudget(2000))
				if function.Name() != "bound" {
					if result.Complete() || result.Reason == "" {
						t.Errorf("%s lost imported requirement: %+v", function, result)
					}
					continue
				}
				if !result.Complete() || len(result.Workers) != 1 || len(result.Workers[0].Operations) != 1 || len(result.Operations) != 1 {
					t.Fatalf("imported bound = %+v", result)
				}
				cancel, receive := result.Operations[0], result.Workers[0].Operations[0]
				if cancel.Kind != Cancel || receive.Kind != Receive || cancel.Resource != receive.Resource {
					t.Errorf("imported identity differs: cancel=%+v receive=%+v", cancel, receive)
				}
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "cancelconsumer")
}
