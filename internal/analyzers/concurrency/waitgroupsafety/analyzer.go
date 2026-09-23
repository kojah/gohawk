// Package waitgroupsafety checks bounded WaitGroup counter underflows.
package waitgroupsafety

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Concurrency: true})

// Analyzer reports proven extra Done operations on fresh, fully modeled groups.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "waitgroupsafety", Doc: "checks proven WaitGroup counter underflows",
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
		probe := trace.For(pass, "waitgroupsafety", string(check.WaitGroupNegativeCounter), function.Pos())
		probe.Candidate(trace.Step{Reason: "counter-summary", Outcome: trace.OutcomeObserved})
		proof := proveCounter(function, engine.Root(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget)))
		outcome := trace.OutcomeUnknown
		if proof.Proven() {
			outcome = trace.OutcomeRejected
		} else if proof.Known() {
			outcome = trace.OutcomeAccepted
		}
		probe.Decision(trace.Step{Reason: string(proof.Reason), Outcome: outcome, Pos: function.Pos()})
		if proof.Proven() {
			reportUnderflow(pass, proof)
		}
	}
	return nil, nil
}

func reportUnderflow(pass *analysis.Pass, proof counterProof) {
	op := proof.operation
	source := syntax.SourceRange(pass, op.Site)
	check.Report(pass, check.WaitGroupNegativeCounter, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(), Message: "WaitGroup Done makes the counter negative",
		Related: []analysis.RelatedInformation{
			{Pos: op.Resource.Value.Pos(), Message: "this fresh WaitGroup starts at zero"},
			{Pos: op.Source, Message: "this Done has no remaining count to discharge"},
		},
	})
}
