// Package goroutineownership implements the goroutineownership gohawk analyzer.
package goroutineownership

import (
	"go/token"
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

type goroutineAnalysis struct {
	pass         *analysis.Pass
	function     *ssa.Function
	spawn        *ssa.Go
	config       goroutineOwnershipConfig
	checkID      check.ID
	signals      []ssa.Value
	groups       []ssa.Value
	owners       []ssa.Value
	testFunction bool
	evidence     *ssaflow.EvidenceQuery
}

// HasExplicitGoroutineOwnership reports whether spawn has a recognized join,
// stop signal, lifecycle owner, or transfer independent of context cancellation.
// Context-policy uses this distinction because context.Background satisfies the
// Context interface but cannot actually stop test-owned asynchronous work.
func HasExplicitGoroutineOwnership(spawn *ssa.Go) bool {
	if spawn == nil || spawn.Parent() == nil {
		return false
	}
	function := spawn.Parent()
	var evidence ssaflow.EvidenceQuery
	signals, groups := goroutineJoinValues(spawn)
	owners := goroutineLifecycleValues(spawn)
	if goroutineHasStopLifecycle(spawn) || goroutineTransferredToCaller(function, spawn) || externallyOwnedLifecycle(owners) ||
		externallyOwnedJoin(signals, groups) ||
		ownershipRegisteredBefore(spawn, signals, groups) ||
		matchingCountedJoin(function, spawn, signals) ||
		nestedCallbackReceivesAny(function, signals) {
		return true
	}
	return !ssaflow.UnownedReturn(spawn, func(candidate ssa.Instruction) bool {
		return joinsGoroutine(candidate, signals, groups) || waitsForLifecycleOwner(&evidence, candidate, owners) ||
			ownsGoroutineLifecycle(&evidence, candidate, owners) ||
			transfersGoroutineOwnership(&evidence, candidate, signals, groups, owners)
	}, func(returned *ssa.Return) bool {
		return ssaflow.ReturnedSameAsAny(returned, signals) || ssaflow.ReturnedSameAsAny(returned, groups) || ssaflow.ReturnedSameAsAny(returned, owners)
	})
}

const (
	goroutineModeContext   = "context"
	goroutineModeLifecycle = "lifecycle"
	goroutineModeJoin      = "join"
)

func runGoroutineOwnership(pass *analysis.Pass, config goroutineOwnershipConfig) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		var evidence ssaflow.EvidenceQuery
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				spawn, ok := instruction.(*ssa.Go)
				if ok {
					analyzeSpawn(pass, function, spawn, config, &evidence)
				}
			}
		}
	}
	return nil, nil
}

func analyzeSpawn(
	pass *analysis.Pass,
	function *ssa.Function,
	spawn *ssa.Go,
	config goroutineOwnershipConfig,
	evidence *ssaflow.EvidenceQuery,
) {
	signals, groups := goroutineJoinValues(spawn)
	owners := goroutineLifecycleValues(spawn)
	checkID := goroutineCheck(config, signals, groups)
	analysis := goroutineAnalysis{
		pass: pass, function: function, spawn: spawn, config: config, checkID: checkID,
		signals: signals, groups: groups, owners: owners,
		testFunction: strings.HasSuffix(pass.Fset.Position(function.Pos()).Filename, "_test.go"),
		evidence:     evidence,
	}
	if reason, accepted := acceptedGoroutineOwnership(function, spawn, config, signals, groups, owners); accepted {
		analysis.emitTrace(spawn.Pos(), reason, analysisTrace.OutcomeAccepted)
		return
	}
	leaks := ssaflow.UnownedReturn(
		spawn,
		analysis.instructionOwnsGoroutine,
		func(returned *ssa.Return) bool {
			return goroutineOwnershipReturned(returned, config, signals, groups, owners)
		},
	)
	outcome, reason := analysisTrace.OutcomeAccepted, "join-proven"
	if leaks {
		outcome, reason = analysisTrace.OutcomeRejected, "unowned-return"
	}
	analysis.emitTrace(spawn.Pos(), reason, outcome)
	if leaks {
		// Without a completion signal or wait group, static analysis cannot
		// reliably distinguish a leak from intentional component work. Keep
		// that heuristic opt-in; reserve the default check for code that
		// exposes a recognizable join mechanism and fails to honor it.
		check.Reportf(pass, checkID, spawn.Pos(), "goroutine is not joined on every return path")
	}
}

func goroutineCheck(config goroutineOwnershipConfig, signals, groups []ssa.Value) check.ID {
	if config.mode != goroutineModeJoin && len(signals) == 0 && len(groups) == 0 {
		return check.GoroutineDetached
	}
	return check.GoroutineJoin
}

func acceptedGoroutineOwnership(
	function *ssa.Function,
	spawn *ssa.Go,
	config goroutineOwnershipConfig,
	signals, groups, owners []ssa.Value,
) (string, bool) {
	// A received channel argument is a stop signal, not a completion handle
	// that the caller must join. Kubernetes informers commonly express context
	// ownership as Run(ctx.Done()):
	// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
	if config.mode == goroutineModeContext && (goroutineHasContextLifecycle(spawn) || goroutineHasStopLifecycle(spawn)) {
		return "context-lifecycle", true
	}
	if config.mode != goroutineModeJoin &&
		(goroutineTransferredToCaller(function, spawn) || externallyOwnedLifecycle(owners) || externallyOwnedJoin(signals, groups)) {
		return "caller-or-external-owner", true
	}
	if ownershipRegisteredBefore(spawn, signals, groups) {
		return "registration-before-spawn", true
	}
	// Matching bounds prove that every launched worker has a corresponding
	// receive without assuming unrelated loops happen to have equal counts.
	if matchingCountedJoin(function, spawn, signals) {
		return "matching-counted-join", true
	}
	if nestedCallbackReceivesAny(function, signals) {
		return "nested-callback-receive", true
	}
	return "", false
}

func (analysis goroutineAnalysis) instructionOwnsGoroutine(candidate ssa.Instruction) bool {
	var reason string
	switch {
	case joinsGoroutine(candidate, analysis.signals, analysis.groups):
		reason = "join-observed"
	case waitsForLifecycleOwner(analysis.evidence, candidate, analysis.owners):
		reason = "lifecycle-wait"
	case analysis.testFunction && causalTestJoin(analysis.spawn, candidate):
		reason = "causal-test-join"
	case analysis.config.mode != goroutineModeJoin && ownsGoroutineLifecycle(analysis.evidence, candidate, analysis.owners):
		reason = "lifecycle-owner"
	case transfersGoroutineOwnership(
		analysis.evidence, candidate, analysis.signals, analysis.groups, ownershipCandidates(analysis.config, analysis.owners),
	):
		reason = "ownership-transfer"
	default:
		return false
	}
	emitGoroutineEvidence(analysis.pass, analysis.function, candidate, analysis.checkID, reason)
	return true
}

func ownershipCandidates(config goroutineOwnershipConfig, owners []ssa.Value) []ssa.Value {
	if config.mode == goroutineModeJoin {
		return nil
	}
	return owners
}

func goroutineOwnershipReturned(
	returned *ssa.Return,
	config goroutineOwnershipConfig,
	signals, groups, owners []ssa.Value,
) bool {
	return ssaflow.ReturnedSameAsAny(returned, signals) ||
		ssaflow.ReturnedSameAsAny(returned, groups) ||
		config.mode != goroutineModeJoin && ssaflow.ReturnedSameAsAny(returned, owners)
}

func emitGoroutineEvidence(pass *analysis.Pass, function *ssa.Function, instruction ssa.Instruction, check check.ID, reason string) {
	if !analysisTrace.Enabled("goroutineownership", string(check)) {
		return
	}
	analysisTrace.Emit(
		pass,
		analysisTrace.Event{
			Analyzer: "goroutineownership",
			Check:    string(check),
			Phase:    "evidence",
			Reason:   reason,
			Outcome:  analysisTrace.OutcomeAccepted,
			Pos:      instruction.Pos(),
			Function: function.String(),
		},
	)
}

func (analysis goroutineAnalysis) emitTrace(
	position token.Pos,
	reason string,
	outcome analysisTrace.Outcome,
) {
	if !analysisTrace.Enabled("goroutineownership", string(analysis.checkID)) {
		return
	}
	analysisTrace.Emit(
		analysis.pass,
		analysisTrace.Event{
			Analyzer: "goroutineownership",
			Check:    string(analysis.checkID),
			Phase:    "decision",
			Reason:   reason,
			Outcome:  outcome,
			Pos:      position,
			Function: analysis.function.String(),
			Details: map[string]string{
				"signals": strconv.Itoa(len(analysis.signals)),
				"groups":  strconv.Itoa(len(analysis.groups)),
				"owners":  strconv.Itoa(len(analysis.owners)),
			},
		},
	)
}
