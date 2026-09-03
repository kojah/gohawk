// Package goroutineownership implements the goroutineownership gohawk analyzer.
package goroutineownership

import (
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/flagvalue"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	config := goroutineOwnershipConfig{mode: goroutineModeContext}
	analyzer := &analysis.Analyzer{
		Name:     "goroutineownership",
		Doc:      "checks that explicit goroutines have a recognizable join handle or lifecycle owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
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
		testFile := strings.HasSuffix(pass.Fset.Position(function.Pos()).Filename, "_test.go")
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				spawn, ok := instruction.(*ssa.Go)
				if !ok {
					continue
				}
				analysis := newSpawnAnalysis(function, spawn, config)
				proof := analysis.prove()
				analysis.emitTrace(pass, proof)
				if analysis.reportable(proof, testFile) {
					check.Reportf(pass, analysis.checkID, spawn.Pos(), "goroutine is not joined on every return path")
				}
			}
		}
	}
	return nil, nil
}

// reportable applies the default/opt-in split. The unjoined check reports only
// a proven violation of an obligation the function itself established. The
// detached check is a heuristic audit: it reports the absence of any
// recognizable lifecycle, but never inside test files, where fixtures and test
// frameworks routinely own workers through shapes this analyzer does not model.
func (analysis *spawnAnalysis) reportable(proof GoroutineProof, testFile bool) bool {
	if proof.Outcome == GoroutineLifecycleViolated {
		return true
	}
	return analysis.checkID == check.GoroutineDetached && proof.Reason == reasonDetachedUnknown && !testFile
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
	// The steps that were tried and did not hold come first, so a reader sees
	// the suppressions this proof ruled out before the reported reason won.
	for _, reason := range analysis.considered {
		probe.Considered(analysisTrace.Step{
			Reason:   string(reason),
			Outcome:  analysisTrace.OutcomeRejected,
			Pos:      analysis.spawn.Pos(),
			Function: analysis.function.String(),
		})
	}
	probe.Decision(analysisTrace.Step{
		Reason:   string(proof.Reason),
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
