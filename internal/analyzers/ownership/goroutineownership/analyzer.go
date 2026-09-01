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
	pass                   *analysis.Pass
	function               *ssa.Function
	spawn                  *ssa.Go
	config                 goroutineOwnershipConfig
	checkID                check.ID
	signals                []ssa.Value
	groups                 []ssa.Value
	owners                 []ssa.Value
	testFunction           bool
	acceptContextLifecycle bool
	evidence               *ssaflow.EvidenceQuery
}

type goroutineOwnershipReason string

const (
	ownershipReasonContextLifecycle        goroutineOwnershipReason = "context-lifecycle"
	ownershipReasonHelperStopLifecycle     goroutineOwnershipReason = "helper-stop-lifecycle"
	ownershipReasonCallerOrExternalOwner   goroutineOwnershipReason = "caller-or-external-owner"
	ownershipReasonRegistrationBeforeSpawn goroutineOwnershipReason = "registration-before-spawn"
	ownershipReasonSynctestBubbleOwner     goroutineOwnershipReason = "synctest-bubble-owner"
	ownershipReasonMatchingCountedJoin     goroutineOwnershipReason = "matching-counted-join"
	ownershipReasonNestedCallbackReceive   goroutineOwnershipReason = "nested-callback-receive"
	ownershipReasonJoinObserved            goroutineOwnershipReason = "join-observed"
	ownershipReasonLifecycleWait           goroutineOwnershipReason = "lifecycle-wait"
	ownershipReasonCausalTestJoin          goroutineOwnershipReason = "causal-test-join"
	ownershipReasonLifecycleOwner          goroutineOwnershipReason = "lifecycle-owner"
	ownershipReasonOwnershipTransfer       goroutineOwnershipReason = "ownership-transfer"
	ownershipReasonReturnedAggregateOwner  goroutineOwnershipReason = "returned-aggregate-lifecycle-owner"
	ownershipReasonJoinProven              goroutineOwnershipReason = "join-proven"
	ownershipReasonUnownedReturn           goroutineOwnershipReason = "unowned-return"
)

type goroutineOwnershipProof struct {
	proven bool
	reason goroutineOwnershipReason
}

// HasExplicitGoroutineOwnership reports whether spawn has a recognized join,
// stop signal, lifecycle owner, or transfer independent of context cancellation.
// Context-policy uses this distinction because context.Background satisfies the
// Context interface but cannot actually stop test-owned asynchronous work.
func HasExplicitGoroutineOwnership(spawn *ssa.Go) bool {
	if spawn == nil || spawn.Parent() == nil {
		return false
	}
	var evidence ssaflow.EvidenceQuery
	ownership := newGoroutineAnalysis(
		nil,
		spawn.Parent(),
		spawn,
		goroutineOwnershipConfig{mode: goroutineModeContext},
		&evidence,
		false,
	)
	return ownership.prove().proven
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
	ownership := newGoroutineAnalysis(pass, function, spawn, config, evidence, true)
	proof := ownership.prove()
	outcome := analysisTrace.OutcomeAccepted
	if !proof.proven {
		outcome = analysisTrace.OutcomeRejected
	}
	ownership.emitTrace(spawn.Pos(), proof.reason, outcome)
	if !proof.proven {
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
	evidence *ssaflow.EvidenceQuery,
	acceptContextLifecycle bool,
) goroutineAnalysis {
	signals, groups := goroutineJoinValues(spawn)
	owners := goroutineLifecycleValues(spawn)
	testFunction := pass != nil && strings.HasSuffix(pass.Fset.Position(function.Pos()).Filename, "_test.go")
	return goroutineAnalysis{
		pass: pass, function: function, spawn: spawn, config: config,
		checkID: goroutineCheck(config, signals, groups),
		signals: signals, groups: groups, owners: owners,
		testFunction:           testFunction,
		acceptContextLifecycle: acceptContextLifecycle,
		evidence:               evidence,
	}
}

func goroutineCheck(config goroutineOwnershipConfig, signals, groups []ssa.Value) check.ID {
	if config.mode != goroutineModeJoin && len(signals) == 0 && len(groups) == 0 {
		return check.GoroutineDetached
	}
	return check.GoroutineJoin
}

func (analysis goroutineAnalysis) prove() goroutineOwnershipProof {
	if proof := analysis.immediateProof(); proof.proven {
		return proof
	}
	leaks := ssaflow.UnownedReturn(analysis.spawn, analysis.instructionOwnsGoroutine, analysis.returnOwnsGoroutine)
	if leaks {
		return goroutineOwnershipProof{reason: ownershipReasonUnownedReturn}
	}
	return goroutineOwnershipProof{proven: true, reason: ownershipReasonJoinProven}
}

func (analysis goroutineAnalysis) immediateProof() goroutineOwnershipProof {
	// A received channel argument is a stop signal, not a completion handle
	// that the caller must join. Kubernetes informers commonly express context
	// ownership as Run(ctx.Done()):
	// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
	if analysis.config.mode == goroutineModeContext &&
		(analysis.acceptContextLifecycle && goroutineHasContextLifecycle(analysis.spawn) || goroutineHasStopLifecycle(analysis.spawn)) {
		return goroutineOwnershipProof{proven: true, reason: ownershipReasonContextLifecycle}
	}
	if analysis.config.mode == goroutineModeContext && goroutineHasHelperStopLifecycle(analysis.spawn) {
		return goroutineOwnershipProof{proven: true, reason: ownershipReasonHelperStopLifecycle}
	}
	if analysis.config.mode != goroutineModeJoin &&
		(goroutineTransferredToCaller(analysis.function, analysis.spawn) || externallyOwnedLifecycle(analysis.owners) ||
			externallyOwnedJoin(analysis.signals, analysis.groups)) {
		return goroutineOwnershipProof{proven: true, reason: ownershipReasonCallerOrExternalOwner}
	}
	if ownershipRegisteredBefore(analysis.spawn, analysis.signals) {
		return goroutineOwnershipProof{proven: true, reason: ownershipReasonRegistrationBeforeSpawn}
	}
	if synctestOwnsGoroutine(analysis.function) {
		return goroutineOwnershipProof{proven: true, reason: ownershipReasonSynctestBubbleOwner}
	}
	// Matching bounds prove that every launched worker has a corresponding
	// receive without assuming unrelated loops happen to have equal counts.
	if matchingCountedJoin(analysis.function, analysis.spawn, analysis.signals) {
		return goroutineOwnershipProof{proven: true, reason: ownershipReasonMatchingCountedJoin}
	}
	if nestedCallbackReceivesAny(analysis.function, analysis.signals) {
		return goroutineOwnershipProof{proven: true, reason: ownershipReasonNestedCallbackReceive}
	}
	return goroutineOwnershipProof{}
}

func (analysis goroutineAnalysis) instructionOwnsGoroutine(candidate ssa.Instruction) bool {
	var reason goroutineOwnershipReason
	switch {
	case joinsGoroutine(candidate, analysis.signals, analysis.groups):
		reason = ownershipReasonJoinObserved
	case waitsForLifecycleOwner(analysis.evidence, candidate, analysis.owners):
		reason = ownershipReasonLifecycleWait
	case analysis.testFunction && causalTestJoin(analysis.spawn, candidate):
		reason = ownershipReasonCausalTestJoin
	case analysis.config.mode != goroutineModeJoin && ownsGoroutineLifecycle(analysis.evidence, candidate, analysis.owners):
		reason = ownershipReasonLifecycleOwner
	case transfersGoroutineOwnership(
		analysis.evidence, candidate, analysis.signals, analysis.groups, ownershipCandidates(analysis.config, analysis.owners),
	):
		reason = ownershipReasonOwnershipTransfer
	default:
		return false
	}
	analysis.emitEvidence(candidate, reason)
	return true
}

func (analysis goroutineAnalysis) returnOwnsGoroutine(returned *ssa.Return) bool {
	if goroutineOwnershipReturned(returned, analysis.config, analysis.signals, analysis.groups, analysis.owners) {
		return true
	}
	if analysis.config.mode == goroutineModeJoin ||
		!returnedAggregateOwnsLifecycle(analysis.function, analysis.spawn, returned, analysis.owners) {
		return false
	}
	analysis.emitEvidence(returned, ownershipReasonReturnedAggregateOwner)
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

func (analysis goroutineAnalysis) emitTrace(
	position token.Pos,
	reason goroutineOwnershipReason,
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
			Reason:   string(reason),
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
