// Package condsafety checks Cond waits with a proven unheld associated mutex.
package condsafety

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Concurrency: true})

// Analyzer reports waits that attempt to unlock a proven unheld mutex.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "condsafety", Doc: "checks Cond waits on proven unlocked mutexes",
		Requires: summaryKnowledge.Requires(), Run: run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	engine, _ := summaryKnowledge.Provider(pass).Concurrency()
	for _, function := range functions {
		if !function.Pos().IsValid() {
			continue
		}
		probe := trace.For(pass, "condsafety", string(check.CondWaitUnlocked), function.Pos())
		probe.Candidate(trace.Step{Reason: "cond-summary", Outcome: trace.OutcomeObserved})
		proof := proveWait(function, engine)
		outcome := trace.OutcomeUnknown
		if proof.Proven() {
			outcome = trace.OutcomeRejected
		} else if proof.Known() {
			outcome = trace.OutcomeAccepted
		}
		probe.Decision(trace.Step{Reason: string(proof.Reason), Outcome: outcome, Pos: function.Pos()})
		if proof.Proven() {
			source := syntax.SourceRange(pass, proof.wait.Site)
			check.Report(pass, check.CondWaitUnlocked, analysis.Diagnostic{
				Pos: source.Pos(), End: source.End(), Message: "Cond.Wait called with its mutex unlocked",
				Related: []analysis.RelatedInformation{
					{Pos: proof.mutex.Value.Pos(), Message: "this mutex starts unlocked"},
					{Pos: proof.wait.Source, Message: "Wait must release this condition's locked mutex"},
				},
			})
		}
	}
	return nil, nil
}
