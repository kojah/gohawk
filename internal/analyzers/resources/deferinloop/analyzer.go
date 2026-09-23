// Package deferinloop implements the deferinloop gohawk analyzer.
package deferinloop

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Results: true, Lifecycle: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "deferinloop",
		Doc:      "checks cleanup defers whose lifetime extends across loop iterations",
		Requires: summaryKnowledge.Requires(),
		Run:      runDeferInLoop,
	}
}

func runDeferInLoop(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	knowledge := summaryKnowledge.Provider(pass)
	for _, function := range functions {
		evidence, _ := knowledge.LifecycleEvidence("deferinloop", string(check.DeferCleanupInLoop))
		for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](function) {
			evidence.ForCandidate(deferred.Pos())
			obligation, ok := deferredObligation(evidence, deferred)
			if !ok {
				continue
			}
			probe := analysisTrace.For(pass, "deferinloop", string(check.DeferCleanupInLoop), deferred.Pos())
			probe.Candidate(analysisTrace.Step{
				Reason: "deferred-cleanup-in-loop", Outcome: analysisTrace.OutcomeObserved, Pos: deferred.Pos(), Function: function.String(),
				Details: map[string]string{"target": obligation.target.String()},
			})
			if resourceLiveAtNextIteration(evidence, knowledge, probe, deferred, obligation) {
				check.Reportf(pass, check.DeferCleanupInLoop, deferred.Pos(), "deferred cleanup runs after the loop instead of after this iteration")
			}
		}
	}
	return nil, nil
}
