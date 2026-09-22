package channelprotocol

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

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
	analyzertest.Run(t, analysistest.TestData(), analyzer, "channelprotocol")
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
				engine.begin(1)
				if got := engine.summaries.Function(function, engine.budget); got.reason != "protocol-budget-exhausted" {
					t.Errorf("limited summary = %+v", got)
				}
				engine.begin(instructionBudget)
				if got := engine.summaries.Function(function, engine.budget); got.reason != "" || len(got.operations) != 2 {
					t.Errorf("fresh summary budget did not recover: %+v", got)
				}
				engine.begin(0)
				if got := engine.summaries.Function(function, engine.budget); got.reason != "" || len(got.operations) != 2 {
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
			engine.begin(instructionBudget)
			summary := engine.summaries.Function(function, engine.budget)
			if summary.reason != "" || len(summary.operations) != 3 || len(summary.deferred) != 0 {
				t.Fatalf("incomplete deferred summary: %+v", summary)
			}
			for index, parameter := range []int{0, 2, 1} {
				op := summary.operations[index]
				kind := closeOperation
				if index == 0 {
					kind = sendOperation
				}
				if op.kind != kind || op.resource.value != function.Params[parameter] {
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
