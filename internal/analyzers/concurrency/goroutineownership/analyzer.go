// Package goroutineownership implements the goroutineownership gohawk analyzer.
package goroutineownership

import (
	"strconv"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/flagvalue"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Results: true, Lifecycle: true, Concurrency: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	config := goroutineOwnershipConfig{mode: goroutineModeContext}
	analyzer := &analysis.Analyzer{
		Name:     "goroutineownership",
		Doc:      "checks that proven goroutine completion obligations are honored",
		Requires: summaryKnowledge.Requires(),
	}
	analyzer.Flags.Var(
		flagvalue.NewChoice(&config.mode, goroutineModeContext, goroutineModeLifecycle, goroutineModeJoin),
		"mode",
		"ownership policy: context, lifecycle, or join",
	)
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runGoroutineOwnership(pass, config)
	}
	return analyzer
}

type goroutineOwnershipConfig struct {
	mode string
}

const (
	// goroutineModeContext also accepts workers bounded by a caller-owned
	// context or stop channel and workers whose lifecycle owner is settled.
	goroutineModeContext = "context"
	// goroutineModeLifecycle accepts settled lifecycle owners but not
	// context or stop-channel boundaries.
	goroutineModeLifecycle = "lifecycle"
	// goroutineModeJoin accepts only an observed completion signal or wait group.
	goroutineModeJoin = "join"
)

func runGoroutineOwnership(pass *analysis.Pass, config goroutineOwnershipConfig) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				spawn, ok := instruction.(*ssa.Go)
				if !ok {
					continue
				}
				analysis := newSpawnAnalysis(pass, function, spawn, config)
				proof := analysis.prove()
				analysis.emitTrace(pass, proof)
				if proof.Outcome == GoroutineLifecycleViolated {
					reportUnjoined(pass, analysis.checkID, spawn, proof)
				}
			}
		}
	}
	return nil, nil
}

// reportUnjoined reports a goroutine whose join is missing on some return,
// citing the return the proof reached without one.
func reportUnjoined(pass *analysis.Pass, id check.ID, spawn *ssa.Go, proof GoroutineProof) {
	source := syntax.SourceRange(pass, spawn.Pos())
	check.Report(pass, id, analysis.Diagnostic{
		Pos:     source.Pos(),
		End:     source.End(),
		Message: "goroutine is not joined on every return path",
		Related: check.ReturnEvidence(pass, proof.Witness, "waiting for the goroutine"),
	})
}

func (analysis *spawnAnalysis) emitTrace(pass *analysis.Pass, proof GoroutineProof) {
	// The spawn is the candidate every step of this proof serves, so selecting
	// it once here skips the whole walk for spawns the reader did not ask about.
	probe := analysisTrace.For(pass, "goroutineownership", string(analysis.checkID), analysis.spawn.Pos())
	if !probe.Enabled() {
		return
	}
	outcome := analysisTrace.OutcomeUnknown
	switch proof.Outcome {
	case GoroutineLifecycleHonored, GoroutineTransferred:
		outcome = analysisTrace.OutcomeAccepted
	case GoroutineLifecycleViolated:
		outcome = analysisTrace.OutcomeRejected
	case GoroutineUnknown:
	}
	for instruction, action := range analysis.actions {
		if action == actionNone {
			continue
		}
		probe.Evidence(analysisTrace.Step{
			Reason:   action.String(),
			Outcome:  analysisTrace.OutcomeAccepted,
			Pos:      instruction.Pos(),
			Function: analysis.function.String(),
			Details:  map[string]string{"instruction": instruction.String()},
		})
	}
	for edge, reason := range analysis.edgeReasons {
		edgeOutcome := analysisTrace.OutcomeAccepted
		if reason == reasonSelectedContextEdge {
			edgeOutcome = analysisTrace.OutcomeUnknown
		}
		probe.Evidence(analysisTrace.Step{
			Reason: reason.String(), Outcome: edgeOutcome,
			Pos: analysis.spawn.Pos(), Function: analysis.function.String(),
			Details: map[string]string{"from_block": strconv.Itoa(edge[0]), "to_block": strconv.Itoa(edge[1])},
		})
	}
	// The steps that were tried and did not hold come first, so a reader sees
	// the suppressions this proof ruled out before the reported reason won.
	for _, reason := range analysis.considered {
		probe.Considered(analysisTrace.Step{
			Reason:   reason.String(),
			Outcome:  analysisTrace.OutcomeRejected,
			Pos:      analysis.spawn.Pos(),
			Function: analysis.function.String(),
		})
	}
	probe.Decision(analysisTrace.Step{
		Reason:   proof.Reason.String(),
		Outcome:  outcome,
		Pos:      analysis.spawn.Pos(),
		Function: analysis.function.String(),
		Details: map[string]string{
			"signals": strconv.Itoa(len(analysis.signals)),
			"groups":  strconv.Itoa(len(analysis.groups)),
			"owners":  strconv.Itoa(len(analysis.owners)),
		},
	})
}
