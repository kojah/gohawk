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
	config := goroutineOwnershipConfig{mode: goroutineModeContext, acceptContextLifecycle: true}
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
	mode                   string
	acceptContextLifecycle bool
}

type goroutineAnalysis struct {
	pass          *analysis.Pass
	function      *ssa.Function
	spawn         *ssa.Go
	config        goroutineOwnershipConfig
	checkID       check.ID
	signals       []ssa.Value
	groups        []ssa.Value
	owners        []ssa.Value
	unsettledDone ssa.Instruction
	testFunction  bool
	evidence      *ssaflow.LocalEvidence
}

type goroutineOwnershipReason string

const (
	ownershipReasonContextLifecycle        goroutineOwnershipReason = "context-lifecycle"
	ownershipReasonStopLifecycle           goroutineOwnershipReason = "stop-lifecycle"
	ownershipReasonHelperStopLifecycle     goroutineOwnershipReason = "helper-stop-lifecycle"
	ownershipReasonCallerOrExternalOwner   goroutineOwnershipReason = "caller-or-external-owner"
	ownershipReasonRegistrationBeforeSpawn goroutineOwnershipReason = "registration-before-spawn"
	ownershipReasonSynctestBubbleOwner     goroutineOwnershipReason = "synctest-bubble-owner"
	ownershipReasonJoinObserved            goroutineOwnershipReason = "join-observed"
	ownershipReasonTestingCleanupJoin      goroutineOwnershipReason = "testing-cleanup-join"
	ownershipReasonDeferredWaitGroupJoin   goroutineOwnershipReason = "deferred-waitgroup-join"
	ownershipReasonOwnershipTransfer       goroutineOwnershipReason = "ownership-transfer"
	ownershipReasonJoinProven              goroutineOwnershipReason = "join-proven"
	ownershipReasonUnownedReturn           goroutineOwnershipReason = "unowned-return"
	ownershipReasonDoneBeforeCompletion    goroutineOwnershipReason = "waitgroup-done-before-completion"
	ownershipReasonDetachedUnknown         goroutineOwnershipReason = "detached-lifecycle-unknown"
	ownershipReasonOpaqueTransfer          goroutineOwnershipReason = "opaque-ownership-transfer"
)

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
		var evidence ssaflow.LocalEvidence
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
	evidence *ssaflow.LocalEvidence,
) {
	ownership := newGoroutineAnalysis(pass, function, spawn, config, evidence)
	proof := ownership.prove()
	if proof.Outcome == GoroutineLifecycleViolated {
		ownership.emitRejectedDoneEvidence()
	}
	ownership.emitTrace(spawn.Pos(), proof)
	report := proof.Outcome == GoroutineLifecycleViolated ||
		ownership.checkID == check.GoroutineDetached && proof.Reason == ownershipReasonDetachedUnknown
	if report {
		// Without a completion signal or wait group, static analysis cannot
		// reliably distinguish a leak from intentional component work. Keep
		// that heuristic opt-in; reserve the default check for code that
		// exposes a recognizable join mechanism and fails to honor it.
		check.Reportf(pass, ownership.checkID, spawn.Pos(), "goroutine is not joined on every return path")
	}
}

func newGoroutineAnalysis(
	pass *analysis.Pass,
	function *ssa.Function,
	spawn *ssa.Go,
	config goroutineOwnershipConfig,
	evidence *ssaflow.LocalEvidence,
) goroutineAnalysis {
	signals, groups, unsettledDone := goroutineJoinValues(spawn)
	owners := goroutineLifecycleValues(spawn)
	testFunction := pass != nil && strings.HasSuffix(pass.Fset.Position(function.Pos()).Filename, "_test.go")
	return goroutineAnalysis{
		pass: pass, function: function, spawn: spawn, config: config,
		checkID: goroutineCheck(config, signals, groups, unsettledDone),
		signals: signals, groups: groups, owners: owners, unsettledDone: unsettledDone,
		testFunction: testFunction,
		evidence:     evidence,
	}
}

func goroutineCheck(config goroutineOwnershipConfig, signals, groups []ssa.Value, unsettledDone ssa.Instruction) check.ID {
	if config.mode != goroutineModeJoin && len(signals) == 0 && len(groups) == 0 && unsettledDone == nil {
		return check.GoroutineDetached
	}
	return check.GoroutineJoin
}

func (analysis goroutineAnalysis) emitRejectedDoneEvidence() {
	if analysis.pass == nil || analysis.unsettledDone == nil ||
		!analysisTrace.Enabled("goroutineownership", string(analysis.checkID)) {
		return
	}
	analysisTrace.Emit(analysis.pass, analysisTrace.Event{
		Analyzer: "goroutineownership",
		Check:    string(analysis.checkID),
		Phase:    "evidence",
		Reason:   string(ownershipReasonDoneBeforeCompletion),
		Outcome:  analysisTrace.OutcomeRejected,
		Pos:      analysis.unsettledDone.Pos(),
		Function: analysis.function.String(),
	})
}

func (analysis goroutineAnalysis) immediateProof() GoroutineProof {
	// A received channel argument is a stop signal, not a completion handle
	// that the caller must join. Kubernetes informers commonly express context
	// ownership as Run(ctx.Done()):
	// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
	if analysis.config.mode == goroutineModeContext && goroutineHasStopLifecycle(analysis.spawn) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: ownershipReasonStopLifecycle}
	}
	if analysis.config.mode == goroutineModeContext && analysis.config.acceptContextLifecycle &&
		goroutineConsumesContextLifecycle(analysis.spawn) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: ownershipReasonContextLifecycle}
	}
	if analysis.config.mode == goroutineModeContext && goroutineHasHelperStopLifecycle(analysis.spawn) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: ownershipReasonHelperStopLifecycle}
	}
	if analysis.config.mode != goroutineModeJoin &&
		externallyOwnedJoin(analysis.signals, analysis.groups) {
		return GoroutineProof{Outcome: GoroutineTransferred, Reason: ownershipReasonCallerOrExternalOwner}
	}
	if ownershipRegisteredBefore(analysis.spawn, analysis.signals) {
		return GoroutineProof{Outcome: GoroutineTransferred, Reason: ownershipReasonRegistrationBeforeSpawn}
	}
	if synctestOwnsGoroutine(analysis.function) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: ownershipReasonSynctestBubbleOwner}
	}
	if deferred := dominatingDeferredWaitGroupJoin(analysis.function, analysis.spawn, analysis.groups); deferred != nil {
		analysis.emitEvidence(deferred, ownershipReasonDeferredWaitGroupJoin)
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: ownershipReasonDeferredWaitGroupJoin}
	}
	return GoroutineProof{}
}

func (analysis goroutineAnalysis) instructionOwnsGoroutine(candidate ssa.Instruction) bool {
	var reason goroutineOwnershipReason
	switch {
	case joinsGoroutine(candidate, analysis.signals, analysis.groups):
		reason = ownershipReasonJoinObserved
	case testingCleanupJoinsGoroutine(candidate, analysis.groups):
		reason = ownershipReasonTestingCleanupJoin
	case transfersGoroutineOwnershipExactly(
		analysis.evidence, candidate, analysis.signals, analysis.groups, ownershipCandidates(analysis.config, analysis.owners),
	):
		reason = ownershipReasonOwnershipTransfer
	default:
		return false
	}
	analysis.emitEvidence(candidate, reason)
	return true
}

func (analysis goroutineAnalysis) instructionOwnsOrAmbiguouslyTransfers(candidate ssa.Instruction) bool {
	return analysis.instructionOwnsGoroutine(candidate) || analysis.instructionMayOwnGoroutine(candidate)
}

func (analysis goroutineAnalysis) instructionMayOwnGoroutine(candidate ssa.Instruction) bool {
	return analysis.config.mode != goroutineModeJoin &&
		(waitsForLifecycleOwner(analysis.evidence, candidate, analysis.owners) ||
			testingCleanupOwnsLaunchedLifecycle(candidate, analysis.spawn) ||
			ownsGoroutineLifecycle(analysis.evidence, candidate, analysis.owners)) ||
		analysis.testFunction && causalTestJoin(analysis.spawn, candidate) ||
		eventuallyJoinsGoroutine(candidate, analysis.signals) ||
		ambiguouslyTransfersGoroutineOwnership(
			analysis.evidence, candidate, analysis.signals, analysis.groups, ownershipCandidates(analysis.config, analysis.owners),
		)
}

func (analysis goroutineAnalysis) returnOwnsGoroutine(returned *ssa.Return) bool {
	return ssaflow.ReturnedSameAsAny(returned, analysis.signals) || ssaflow.ReturnedSameAsAny(returned, analysis.groups)
}

func (analysis goroutineAnalysis) returnMayOwnGoroutine(returned *ssa.Return) bool {
	if analysis.returnOwnsGoroutine(returned) {
		return true
	}
	return analysis.config.mode != goroutineModeJoin &&
		(ssaflow.ReturnedSameAsAny(returned, analysis.owners) ||
			returnedAggregateOwnsLifecycle(analysis.function, analysis.spawn, returned, analysis.owners))
}

func ownershipCandidates(config goroutineOwnershipConfig, owners []ssa.Value) []ssa.Value {
	if config.mode == goroutineModeJoin {
		return nil
	}
	return owners
}

func (analysis goroutineAnalysis) emitEvidence(instruction ssa.Instruction, reason goroutineOwnershipReason) {
	if analysis.pass == nil {
		return
	}
	emitGoroutineEvidence(analysis.pass, analysis.function, instruction, analysis.checkID, reason)
}

func emitGoroutineEvidence(
	pass *analysis.Pass,
	function *ssa.Function,
	instruction ssa.Instruction,
	check check.ID,
	reason goroutineOwnershipReason,
) {
	if !analysisTrace.Enabled("goroutineownership", string(check)) {
		return
	}
	analysisTrace.Emit(
		pass,
		analysisTrace.Event{
			Analyzer: "goroutineownership",
			Check:    string(check),
			Phase:    "evidence",
			Reason:   string(reason),
			Outcome:  analysisTrace.OutcomeAccepted,
			Pos:      instruction.Pos(),
			Function: function.String(),
		},
	)
}

func (analysis goroutineAnalysis) emitTrace(position token.Pos, proof GoroutineProof) {
	if !analysisTrace.Enabled("goroutineownership", string(analysis.checkID)) {
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
	analysisTrace.Emit(
		analysis.pass,
		analysisTrace.Event{
			Analyzer: "goroutineownership",
			Check:    string(analysis.checkID),
			Phase:    "decision",
			Reason:   string(proof.Reason),
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
