package goroutineownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// This file owns the single decision path for every goroutineownership check.
// The proof reverses the burden of evidence: the worker must first establish an
// obligation (a completion signal or a settling WaitGroup). A diagnostic then
// needs a feasible return path on which nothing joins, transfers, or ambiguously
// consumes it.
// Every instruction after the spawn is classified once; the shared obligation
// walk then reports whether exact actions, only opaque ones, or nothing at all
// covers every return.

// GoroutineOutcome distinguishes proven lifecycle behavior from an opaque
// handoff. Unknown evidence suppresses correctness diagnostics.
type GoroutineOutcome uint8

const (
	GoroutineUnknown GoroutineOutcome = iota
	GoroutineLifecycleHonored
	GoroutineLifecycleViolated
	GoroutineTransferred
)

// GoroutineProof is the single result consumed by reporting, tracing, and
// cross-analyzer ownership queries.
type GoroutineProof struct {
	Outcome GoroutineOutcome
	Reason  goroutineOwnershipReason
	// Witness is the return a violated proof reached without a join.
	Witness *ssa.Return
}

// ruledOut records a proof step that was evaluated and did not hold. The
// reason names the conclusion that failed, so a trace reader can see which
// suppressions were tried before the reported one won. The two grouped
// helpers above record only the outcomes they prove, because each covers
// several conclusions and a single failure code would not say which.
func (analysis *spawnAnalysis) ruledOut(reason goroutineOwnershipReason) {
	if analysis.tracing {
		analysis.considered = append(analysis.considered, reason)
	}
}

func (analysis *spawnAnalysis) prove() GoroutineProof {
	// A cutoff may hide an alternative completion handle. Partial discovery
	// cannot justify a defect, even when it already found one obligation.
	if analysis.discoveryBudget.Exhausted() {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonDiscoveryBudgetExhausted}
	}
	// Absence of a recognizable owner is not evidence of a defect. This also
	// applies in join mode: a policy setting cannot create a completion promise.
	if len(analysis.signals) == 0 && len(analysis.groups) == 0 {
		// Early Done may announce readiness rather than completion. Without a
		// separate completion promise, later work does not establish a defect.
		// https://github.com/nadoo/glider/blob/38b34030bc0664b958f9226a51d9400258e5d852/dns/server.go#L39-L62
		if analysis.unsettledDone != nil {
			return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonDoneBeforeCompletion}
		}
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonNoObligation}
	}
	if proof, decided := analysis.lifecycleProof(); decided {
		return proof
	}
	if proof, decided := analysis.dominatingProof(); decided {
		return proof
	}
	if analysis.otherWorkerConsumesSignal() {
		// A producer whose channel is drained by worker goroutines launched in
		// the same function hands its completion to those workers: the parent
		// never established a receive of its own to skip. Worker pools launch
		// the consumers in a loop, so dominance cannot credit them. Grafana's
		// alert generator and NetBox's SNMP probe runner use this shape:
		// https://github.com/grafana/alerting/blob/46847d9b586c46b06f8c666a93250ed062e4efb9/testing/alerting-gen/pkg/execute/run.go#L95-L160
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonWorkerConsumesSignal}
	}
	analysis.ruledOut(reasonWorkerConsumesSignal)
	// One walk carries every label to every feasible return: honored when
	// exact joins and transfers cover them all, uncertain when some return is
	// reached only through an opaque handoff, violated when a return is
	// reached with no action at all. Opacity on one path never excuses an
	// unrelated early return.
	outcome, witness := ssapath.EvaluateObligationWitness(ssapath.ObligationFlow{
		Start: analysis.spawn, Instruction: analysis.obligation, Edge: analysis.edgeObligation,
		Successors: summaryKnowledge.Provider(analysis.pass).Successors(), Terminates: summaryKnowledge.Provider(analysis.pass).Terminates(),
	})
	if outcome == ssapath.ObligationHonored {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonJoinProven}
	}
	analysis.ruledOut(reasonJoinProven)
	if analysis.guardedLocalJoin() {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonGuardedLocalJoin}
	}
	analysis.ruledOut(reasonGuardedLocalJoin)
	if outcome == ssapath.ObligationUncertain {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}
	}
	analysis.ruledOut(reasonOpaqueTransfer)
	if analysis.flagGuardedJoin() {
		// A join guarded by a local Boolean that the function assigns around
		// the spawn, such as `started = true` before launching and `if started
		// { wg.Wait() }` after, correlates the guard with the launch in a way
		// this proof does not model. Soperator and gocoin both use the shape:
		// https://github.com/nebius/soperator/blob/3f5635c08fab3578db574a55b293b5aa32042bd3/internal/exporter/exporter.go#L157-L193
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonFlagGuardedJoin}
	}
	analysis.ruledOut(reasonFlagGuardedJoin)
	if analysis.countedJoin() {
		// When the spawn itself runs in a loop, a later receive loop that may
		// run zero times is not a proven skip: the spawn loop may have run zero
		// times too. Matching those counts is not modeled, so counted joins stay
		// unknown. A single spawn joined only inside a conditional loop body has
		// no such symmetry and remains reportable.
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonLoopJoinUnproven}
	}
	analysis.ruledOut(reasonLoopJoinUnproven)
	if analysis.sharedStorageSignals() {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonSharedStorageSignal}
	}
	analysis.ruledOut(reasonSharedStorageSignal)
	if analysis.bufferedSignals() {
		// A buffered completion send lets the worker finish after the caller
		// stops receiving, so it does not by itself establish a join protocol.
		// Buildkite uses a one-slot result channel to let its collector finish:
		// https://github.com/buildkite/agent/blob/e206ddf806af50a1ba8c9a6dd501dfda0b730818/internal/artifact/downloader.go#L96-L177
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonBufferedSignal}
	}
	analysis.ruledOut(reasonBufferedSignal)
	censusBudget := analysis.budget()
	unobserved := analysis.proveUnobservedSignalsWithin(censusBudget)
	if !unobserved.Known() {
		if unobserved.Reason == proofs.EvidenceBudgetExhausted {
			return analysis.lifetimeCutoff(censusBudget, querySignalCensus, reasonSignalCensusUnavailable)
		}
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonSignalCensusUnavailable}
	}
	if unobserved.Proven() {
		// A channel the worker only closes, and that nothing in the function
		// or its callees ever receives from, selects on, or hands away, is
		// not a completion protocol: no code waits for it, and close never
		// blocks the worker. Its presence proves no obligation the parent
		// could skip. agentsh's test drain loops close such a channel:
		// https://github.com/canyonroad/agentsh/blob/0ce9939b6ccead8b21b9ce16783b287d18012777/internal/db/proxy/postgres/upstreamread_test.go#L229-L237
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonUnobservedSignal}
	}
	analysis.ruledOut(reasonUnobservedSignal)
	if ssacall.RunsOnceInProgramEntry(analysis.spawn) {
		// A worker launched at most once by main.main cannot accumulate, and
		// every way out of main ends the process and stops the worker. This
		// settles the join obligation only; it is unknown rather than
		// honored, so no other analyzer may read it as proof that the worker
		// finished before a later instruction.
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonProcessExitStopsWorker}
	}
	analysis.ruledOut(reasonProcessExitStopsWorker)
	return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: reasonUnownedReturn, Witness: witness}
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
	relayBudget := analysis.budget()
	relay := analysis.relayDependencyUncertain(relayBudget)
	if relayBudget.Exhausted() {
		return analysis.lifetimeCutoff(relayBudget, queryRelayDependency, reasonRelayDependencyBudgetExhausted), true
	}
	if relay {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonRelayDependency}, true
	}
	// Completion-handle ownership is independent of stop/context inputs. Keep
	// the existing factory-opacity boundary before an external transfer claim;
	// a possible lifetime bound must not replace either contract with a join.
	if proof, decided := analysis.completionHandleProof(); decided {
		return proof, true
	}
	if proof, decided := analysis.callerLifetimeProof(); decided {
		return proof, true
	}
	if synctestOwnsGoroutine(analysis.function) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonSynctestBubbleOwner}, true
	}
	return GoroutineProof{}, false
}

// Every lifetime query shares one allowance. Check availability before naming
// its witness: cutoff in a field-write or callback scan is not a caller bound.
func (analysis *spawnAnalysis) callerLifetimeProof() (GoroutineProof, bool) {
	budget := analysis.budget()
	queries := []struct {
		reason goroutineOwnershipReason
		find   func() bool
	}{
		{reasonStopLifecycle, func() bool { return goroutineReceivesCallerSignal(analysis.pass, analysis.spawn, budget) }},
		{reasonContextLifecycle, func() bool { return goroutineReceivesCallerContext(analysis.pass, analysis.spawn, budget) }},
		{reasonLocallyCanceledContext, func() bool { return goroutineReceivesLocallyCanceledContext(analysis.pass, analysis.spawn, budget) }},
		{reasonReceiverContext, func() bool { return goroutineReceivesReceiverContext(analysis.pass, analysis.spawn, budget) }},
	}
	for _, query := range queries {
		found := query.find()
		if budget.Exhausted() {
			return analysis.lifetimeCutoff(budget, queryCallerLifetime, reasonReceiveBudgetExhausted), true
		}
		if found {
			return GoroutineProof{Outcome: GoroutineUnknown, Reason: query.reason}, true
		}
	}
	return GoroutineProof{}, false
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
