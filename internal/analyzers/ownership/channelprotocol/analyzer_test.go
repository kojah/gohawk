package channelprotocol

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func newSummaryEngine() *summaryEngine { return &summaryEngine{Engine: concurrencyfacts.NewEngine()} }

func (engine *summaryEngine) prove(function *ssa.Function, limit int) cycleProof {
	return engine.proveCheck(function, limit, check.ChannelProtocolBlocked)
}

func TestAnalyzer(t *testing.T) {
	analyzer := Analyzer()
	original := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if len(diagnostic.Related) != 3 {
				t.Errorf("related locations = %d, want 3", len(diagnostic.Related))
			}
			for _, related := range diagnostic.Related {
				if !related.Pos.IsValid() || related.Message == "" {
					t.Errorf("incomplete related evidence: %+v", related)
				}
			}
			report(diagnostic)
		}
		return original(pass)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "channelprotocol", "mixedcycles")
}

func TestSummaryBudgets(t *testing.T) {
	analyzer := Analyzer()
	original := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		functions, err := ssaflow.SourceSSAFunctions(pass)
		if err != nil {
			return nil, err
		}
		checked := 0
		for _, function := range functions {
			switch function.Name() {
			case "direct", "groupCycle", "deferredCycle":
				engine := newSummaryEngine()
				if proof := engine.prove(function, 1); proof.Known() || proof.Reason != "protocol-budget-exhausted" {
					t.Errorf("limited root proof = %+v", proof)
				}
				if proof := engine.prove(function, instructionBudget); !proof.Proven() {
					t.Errorf("fresh root budget did not recover: %+v", proof)
				}
				checked++
			case "worker", "groupWorker", "deferredWorker":
				engine := newSummaryEngine()
				budget := ssaflow.NewSearchBudget(1)
				if got := engine.Function(function, budget); got.Reason != "protocol-budget-exhausted" {
					t.Errorf("limited summary = %+v", got)
				}
				budget = ssaflow.NewSearchBudget(instructionBudget)
				if got := engine.Function(function, budget); got.Reason != "" || len(got.Operations) != 2 {
					t.Errorf("fresh summary budget did not recover: %+v", got)
				}
				budget = ssaflow.NewSearchBudget(0)
				if got := engine.Function(function, budget); got.Reason != "" || len(got.Operations) != 2 {
					t.Errorf("completed summary not cached: %+v", got)
				}
				checked++
			}
		}
		if checked != 6 {
			t.Errorf("checked %d functions, want 6", checked)
		}
		return original(pass)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "channelprotocol")
}

func TestDeferredSummaryOrder(t *testing.T) {
	analyzer := Analyzer()
	original := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		functions, err := ssaflow.SourceSSAFunctions(pass)
		if err != nil {
			return nil, err
		}
		found := false
		for _, function := range functions {
			if function.Name() != "deferredOrderWorker" {
				continue
			}
			found = true
			engine := newSummaryEngine()
			budget := ssaflow.NewSearchBudget(instructionBudget)
			summary := engine.Function(function, budget)
			if summary.Reason != "" || len(summary.Operations) != 3 {
				t.Fatalf("incomplete deferred summary: %+v", summary)
			}
			for index, parameter := range []int{0, 2, 1} {
				op := summary.Operations[index]
				kind := closeOperation
				if index == 0 {
					kind = sendOperation
				}
				if op.Kind != kind || op.Resource.Value != function.Params[parameter] {
					t.Errorf("operation %d = %+v, want kind %d on parameter %d", index, op, kind, parameter)
				}
			}
		}
		if !found {
			t.Fatal("deferred order fixture missing")
		}
		return original(pass)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "channelprotocol")
}
