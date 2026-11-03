package goroutineownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/analysis/analyzers/concurrency/goroutineownership/proofpolicy"

	"github.com/kojah/gohawk/internal/analysis/analyzers/concurrency/goroutineownership/origins"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// This file owns the single decision path for every goroutineownership check.
// The proof reverses the burden of evidence: the worker must first establish an
// obligation (a completion signal or a settling WaitGroup). A diagnostic then
// needs a feasible return path on which nothing joins, transfers, or ambiguously
// consumes it.
// Every instruction after the spawn is classified once; the shared obligation
// walk then reports whether exact actions, only opaque ones, or nothing at all
// covers every return. proofpolicy owns the ordered decisions; this file owns
// search execution, shared allowances and the timing of trace observations.

// GoroutineOutcome retains the public ownership vocabulary used by reporting.
type GoroutineOutcome = proofpolicy.Outcome

const (
	GoroutineUnknown           = proofpolicy.Unknown
	GoroutineLifecycleHonored  = proofpolicy.Honored
	GoroutineLifecycleViolated = proofpolicy.Violated
	GoroutineTransferred       = proofpolicy.Transferred
)

// GoroutineProof is the authoritative decision shared with other analyzers.
type GoroutineProof = proofpolicy.Proof

// ruledOut records a proof step that was evaluated and did not hold. The
// reason names the conclusion that failed, so a trace reader can see which
// suppressions were tried before the reported one won. Composite lifetime
// and dominance queries retain their own multi-conclusion evidence; only the
// proposition adapters below record these individual negative conclusions.
func (analysis *spawnAnalysis) ruledOut(reason goroutineOwnershipReason) {
	if analysis.tracing {
		analysis.considered = append(analysis.considered, reason)
	}
}

func (analysis *spawnAnalysis) prove() GoroutineProof {
	discovery := proofpolicy.Discovery{
		Exhausted:  analysis.discoveryBudget.Exhausted(),
		Obligation: len(analysis.signals) != 0 || len(analysis.groups) != 0,
		EarlyDone:  analysis.unsettledDone != nil,
	}
	return proofpolicy.Prove(discovery, proofQueries{analysis})
}

// otherWorkerConsumesSignal reports whether a worker other than the spawn
// captures or receives a tracked signal: a goroutine launched anywhere in the
// function, or a literal handed to a call, since a runner such as
// errgroup.Go executes it as a worker the analysis cannot see. Iceberg feeds
// a jobs channel from one goroutine and drains it from errgroup workers:
// https://github.com/apache/iceberg-go/blob/aa76a28c34787f23f8eee1c5648271fc0ee6042f/table/table.go#L380-L415
func (analysis *spawnAnalysis) otherWorkerConsumesSignal() bool {
	if len(analysis.signals) == 0 {
		return false
	}
	for _, block := range analysis.function.Blocks {
		for _, instruction := range block.Instrs {
			if instruction == analysis.spawn {
				continue
			}
			switch typed := instruction.(type) {
			case *ssa.Go:
				common := typed.Common()
				if analysis.anyArgumentConsumes(common) || analysis.closureConsumes(common.Value) {
					return true
				}
			case *ssa.Call:
				if slices.ContainsFunc(typed.Common().Args, analysis.closureConsumes) {
					return true
				}
			}
		}
	}
	return false
}

// lifecycleProof distinguishes an exact caller-owned completion handle from a
// possible caller lifetime bound before local flow. A received stop signal or
// context can explain ownership, but cannot prove the worker has been joined.
func (analysis *spawnAnalysis) lifecycleProof() (GoroutineProof, bool) {
	return proofpolicy.Lifecycle(proofQueries{analysis})
}

// Every lifetime query shares one allowance. Check availability before naming
// its witness: cutoff in a field-write or callback scan is not a caller bound.
func (analysis *spawnAnalysis) callerLifetimeProof() (GoroutineProof, bool) {
	return proofpolicy.CallerLifetime(callerLifetimeQueries{analysis: analysis, budget: analysis.budget()})
}

// All four observations share one allowance. Read cutoff after the native
// search returns; a partial witness cannot establish a caller lifetime bound.
type callerLifetimeQueries struct {
	analysis *spawnAnalysis
	budget   *proofs.SearchBudget
}

func (queries callerLifetimeQueries) Stop() proofpolicy.LifetimeObservation {
	found := goroutineReceivesCallerSignal(queries.analysis.pass, queries.analysis.spawn, queries.budget)
	return proofpolicy.LifetimeObservation{Found: found, Interrupted: queries.budget.Exhausted()}
}

func (queries callerLifetimeQueries) Context() proofpolicy.LifetimeObservation {
	found := goroutineReceivesCallerContext(queries.analysis.pass, queries.analysis.spawn, queries.budget)
	return proofpolicy.LifetimeObservation{Found: found, Interrupted: queries.budget.Exhausted()}
}

func (queries callerLifetimeQueries) Canceled() proofpolicy.LifetimeObservation {
	found := goroutineReceivesLocallyCanceledContext(queries.analysis.pass, queries.analysis.spawn, queries.budget)
	return proofpolicy.LifetimeObservation{Found: found, Interrupted: queries.budget.Exhausted()}
}

func (queries callerLifetimeQueries) Receiver() proofpolicy.LifetimeObservation {
	found := goroutineReceivesReceiverContext(queries.analysis.pass, queries.analysis.spawn, queries.budget)
	return proofpolicy.LifetimeObservation{Found: found, Interrupted: queries.budget.Exhausted()}
}

func (queries callerLifetimeQueries) Group() proofpolicy.LifetimeObservation {
	found := goroutineBoundedByExternalWaitGroup(queries.analysis.pass, queries.analysis.spawn, queries.budget)
	return proofpolicy.LifetimeObservation{Found: found, Interrupted: queries.budget.Exhausted()}
}

func (queries callerLifetimeQueries) Cutoff() GoroutineProof {
	return queries.analysis.lifetimeCutoff(queries.budget, queryCallerLifetime, reasonReceiveBudgetExhausted)
}

func (analysis *spawnAnalysis) lifetimeCutoff(
	budget *proofs.SearchBudget, phase queryPhase, reason goroutineOwnershipReason,
) GoroutineProof {
	budget.Observe(proofs.EvidenceBudgetExhausted, analysis.spawn.Pos(), func() map[string]string {
		return map[string]string{"phase": phase.String()}
	})
	return GoroutineProof{Outcome: GoroutineUnknown, Reason: reason}
}

// dominatingProof classifies the instructions that run before every spawn. A
// deferred join registered on every path to the spawn runs after the worker
// settles, as in Zap's pool race test:
// https://github.com/uber-go/zap/blob/bb1a55dd13257cf7cbd06b4146674c67ca614dea/internal/pool/pool_test.go#L85-L105
// A transfer or opaque handoff before the spawn likewise settles the
// obligation before this function could observe completion.
func (analysis *spawnAnalysis) dominatingProof() (GoroutineProof, bool) {
	unknown := false
	budget := analysis.budget()
	for instruction := range cfg.InstructionsStrictlyDominatingWithin(analysis.spawn, budget) {
		_, deferred := instruction.(*ssa.Defer)
		// Testing callbacks also execute later, even when registration
		// precedes the spawn. An ordinary Wait before spawn still cannot join.
		// https://github.com/miniscruff/changie/blob/e78b7fcae4fd76fc588b6442117ef99c39835e15/then/write.go#L19-L35
		deferred = deferred || ssacall.HasLibraryContract(ssaflow.InstructionCall(instruction), ssacall.ContractTestingCleanup)
		switch analysis.action(instruction) {
		case actionJoin:
			if deferred {
				return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonDeferredJoinBeforeSpawn}, true
			}
		case actionTransfer:
			return GoroutineProof{Outcome: GoroutineTransferred, Reason: reasonOwnershipTransfer}, true
		case actionUnknown:
			unknown = true
		case actionNone:
		}
	}
	if budget.Exhausted() {
		return analysis.lifetimeCutoff(budget, queryPreSpawnCensus, reasonPreSpawnCensusCutoff), true
	}
	if unknown {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}, true
	}
	return GoroutineProof{}, false
}

// guardedLocalJoin re-runs the exact flow query with the fact that a channel
// created once before the spawn is non-nil afterwards. An optional worker is
// commonly stopped and waited beneath `if stop != nil`; the launch proves that
// guard true on every path that reaches it. Rainier:
// https://github.com/tokencanopy/rainier/blob/855b2e7c276a60a2f65f141d1071cf03be38d6e3/internal/attachio/attachio.go#L267-L287
func (analysis *spawnAnalysis) guardedLocalJoin() bool {
	if len(analysis.signals)+len(analysis.groups) == 0 {
		return false
	}
	for _, created := range analysis.channelsCreatedOnceBeforeSpawn() {
		// Edge-local joins are deliberately not consulted here; this query
		// asks only whether the non-nil fact lets ordinary exact actions
		// cover every return.
		honored := ssapath.EvaluateObligation(ssapath.ObligationFlow{
			Start: analysis.spawn, NonNil: created, Instruction: analysis.obligation,
			Successors: summaryKnowledge.Provider(analysis.pass).Successors(), Terminates: summaryKnowledge.Provider(analysis.pass).Terminates(),
		}) == ssapath.ObligationHonored
		if honored {
			return true
		}
	}
	return false
}

// channelsCreatedOnceBeforeSpawn returns captured cells with stable contents
// from a MakeChan outside any loop. Mutation or address escape means the guard
// may observe a different channel instance than the worker.
func (analysis *spawnAnalysis) channelsCreatedOnceBeforeSpawn() []ssa.Value {
	closure, ok := analysis.spawn.Common().Value.(*ssa.MakeClosure)
	if !ok || cfg.BlockInCycle(analysis.spawn.Block()) {
		return nil
	}
	var created []ssa.Value
	for _, binding := range closure.Bindings {
		stored := heapmodel.NewStorage(analysis.budget()).StableContent(binding, analysis.spawn)
		channel, ok := stored.Value.(*ssa.MakeChan)
		if stored.Proven() && ok && channel.Parent() == analysis.function && !cfg.BlockInCycle(channel.Block()) {
			// The guard reads the captured cell after launch. StableContent
			// rules out replacement, but its earlier nil state must not become
			// a may-alias assumption in the shared non-nil flow query.
			for _, instruction := range cfg.InstructionsReachableAfter(analysis.spawn) {
				value, ok := instruction.(ssa.Value)
				if !ok {
					continue
				}
				if source, ok := ssaflow.IdentitySource(value); ok && source == binding {
					created = append(created, value)
				}
			}
		}
	}
	return created
}

func (analysis *spawnAnalysis) completionHandleProof() (GoroutineProof, bool) {
	for _, tracked := range analysis.tracked {
		budget := analysis.budget()
		if tracked.kind == trackedSignal {
			if proof, decided := analysis.factoryOriginDecision(origins.Signal(tracked.value, analysis.spawn, budget), budget); decided {
				return proof, true
			}
		}
		if tracked.kind != trackedOwner && ssaflow.ExternallyOwnedValue(tracked.value) {
			return GoroutineProof{Outcome: GoroutineTransferred, Reason: reasonCallerOrExternalOwner}, true
		}
		if tracked.kind == trackedGroup {
			if proof, decided := analysis.factoryOriginDecision(origins.Group(tracked.value, budget), budget); decided {
				return proof, true
			}
		}
	}
	return GoroutineProof{}, false
}

func (analysis *spawnAnalysis) factoryOriginDecision(origin proofs.Proof, budget *proofs.SearchBudget) (GoroutineProof, bool) {
	if !origin.Known() {
		return analysis.lifetimeCutoff(budget, queryFactoryOrigin, reasonFactoryOriginBudgetExhausted), true
	}
	if origin.Proven() {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}, true
	}
	return GoroutineProof{}, false
}

// proofQueries owns evidence execution and trace timing, not the order of
// policy choices. Negative considered events commit before the next query.
type proofQueries struct{ analysis *spawnAnalysis }

func (queries proofQueries) Relay() proofpolicy.Relay {
	budget := queries.analysis.budget()
	found := queries.analysis.relayDependencyUncertain(budget)
	if budget.Exhausted() {
		return proofpolicy.Relay{Kind: proofpolicy.RelayInterrupted, Budget: budget}
	}
	if found {
		return proofpolicy.Relay{Kind: proofpolicy.RelayRelated}
	}
	return proofpolicy.Relay{Kind: proofpolicy.RelayAbsent}
}

func (queries proofQueries) RelayCutoff(budget *proofs.SearchBudget) GoroutineProof {
	return queries.analysis.lifetimeCutoff(budget, queryRelayDependency, reasonRelayDependencyBudgetExhausted)
}

func (queries proofQueries) CompletionHandle() (GoroutineProof, bool) {
	return queries.analysis.completionHandleProof()
}

func (queries proofQueries) CallerLifetime() (GoroutineProof, bool) {
	return queries.analysis.callerLifetimeProof()
}

func (queries proofQueries) Synctest() bool { return synctestOwnsGoroutine(queries.analysis.function) }

func (queries proofQueries) Dominating() (GoroutineProof, bool) {
	return queries.analysis.dominatingProof()
}

func (queries proofQueries) observed(reason goroutineOwnershipReason, found bool) bool {
	if !found {
		queries.analysis.ruledOut(reason)
	}
	return found
}

func (queries proofQueries) Workers() bool {
	return queries.observed(reasonWorkerConsumesSignal, queries.analysis.otherWorkerConsumesSignal())
}

func (queries proofQueries) Flow() proofpolicy.Flow {
	analysis := queries.analysis
	outcome, witness := ssapath.EvaluateObligationWitness(ssapath.ObligationFlow{
		Start: analysis.spawn, Instruction: analysis.obligation, Edge: analysis.edgeObligation,
		Successors: summaryKnowledge.Provider(analysis.pass).Successors(), Terminates: summaryKnowledge.Provider(analysis.pass).Terminates(),
	})
	return proofpolicy.Flow{Outcome: outcome, Witness: witness}
}

func (queries proofQueries) Honored(flow proofpolicy.Flow) bool {
	return queries.observed(reasonJoinProven, flow.Outcome == ssapath.ObligationHonored)
}

func (queries proofQueries) Guarded() bool {
	return queries.observed(reasonGuardedLocalJoin, queries.analysis.guardedLocalJoin())
}

func (queries proofQueries) Uncertain(flow proofpolicy.Flow) bool {
	return queries.observed(reasonOpaqueTransfer, flow.Outcome == ssapath.ObligationUncertain)
}

func (queries proofQueries) FlagGuarded() bool {
	return queries.observed(reasonFlagGuardedJoin, queries.analysis.flagGuardedJoin())
}

func (queries proofQueries) Counted() bool {
	return queries.observed(reasonLoopJoinUnproven, queries.analysis.countedJoin())
}

func (queries proofQueries) SharedStorage() bool {
	return queries.observed(reasonSharedStorageSignal, queries.analysis.sharedStorageSignals())
}

func (queries proofQueries) Buffered() bool {
	return queries.observed(reasonBufferedSignal, queries.analysis.bufferedSignals())
}

func (queries proofQueries) Census() proofpolicy.Census {
	analysis := queries.analysis
	budget := analysis.budget()
	proof := analysis.proveUnobservedSignalsWithin(budget)
	if !proof.Known() {
		if proof.Reason == proofs.EvidenceBudgetExhausted {
			return proofpolicy.Census{Kind: proofpolicy.CensusInterrupted, Budget: budget}
		}
		return proofpolicy.Census{Kind: proofpolicy.CensusUnavailable}
	}
	if proof.Proven() {
		return proofpolicy.Census{Kind: proofpolicy.CensusUnobserved}
	}
	analysis.ruledOut(reasonUnobservedSignal)
	return proofpolicy.Census{Kind: proofpolicy.CensusObserved}
}

func (queries proofQueries) CensusCutoff(budget *proofs.SearchBudget) GoroutineProof {
	return queries.analysis.lifetimeCutoff(budget, querySignalCensus, reasonSignalCensusUnavailable)
}

func (queries proofQueries) ProgramEntry() bool {
	return queries.observed(reasonProcessExitStopsWorker, ssacall.RunsOnceInProgramEntry(queries.analysis.spawn))
}

func (queries proofQueries) Lifecycle() (GoroutineProof, bool) {
	return queries.analysis.lifecycleProof()
}

func (queries proofQueries) Fallback(witness *ssa.Return) GoroutineProof {
	return proofpolicy.Fallback(queries, witness)
}
