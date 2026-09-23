package goroutineownership

import (
	"github.com/kojah/gohawk/internal/ssaflow"

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

type goroutineOwnershipReason string

const (
	reasonJoinProven              goroutineOwnershipReason = "join-proven"
	reasonDeferredJoinBeforeSpawn goroutineOwnershipReason = "deferred-join-before-spawn"
	reasonGuardedLocalJoin        goroutineOwnershipReason = "guarded-local-join"
	reasonStopLifecycle           goroutineOwnershipReason = "stop-lifecycle"
	reasonContextLifecycle        goroutineOwnershipReason = "context-lifecycle"
	reasonLocallyCanceledContext  goroutineOwnershipReason = "locally-canceled-context"
	reasonReceiverContext         goroutineOwnershipReason = "receiver-context-lifecycle"
	reasonRelayDependency         goroutineOwnershipReason = "relay-dependency-lifecycle"
	reasonSynctestBubbleOwner     goroutineOwnershipReason = "synctest-bubble-owner"
	reasonCallerOrExternalOwner   goroutineOwnershipReason = "caller-or-external-owner"
	reasonOwnershipTransfer       goroutineOwnershipReason = "ownership-transfer"
	reasonOpaqueTransfer          goroutineOwnershipReason = "opaque-ownership-transfer"
	reasonLoopJoinUnproven        goroutineOwnershipReason = "loop-join-unproven"
	reasonWorkerConsumesSignal    goroutineOwnershipReason = "signal-consumed-by-worker"
	reasonFlagGuardedJoin         goroutineOwnershipReason = "flag-guarded-join"
	reasonBufferedSignal          goroutineOwnershipReason = "buffered-completion-signal"
	reasonSharedStorageSignal     goroutineOwnershipReason = "shared-storage-signal"
	reasonNoObligation            goroutineOwnershipReason = "no-completion-obligation"
	reasonUnownedReturn           goroutineOwnershipReason = "unowned-return"
	reasonDoneBeforeCompletion    goroutineOwnershipReason = "waitgroup-done-before-completion"
)

// GoroutineProof is the single result consumed by reporting, tracing, and
// cross-analyzer ownership queries.
type GoroutineProof struct {
	Outcome GoroutineOutcome
	Reason  goroutineOwnershipReason
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
	outcome := ssaflow.EvaluateObligation(ssaflow.ObligationFlow{
		Start: analysis.spawn, Instruction: analysis.obligation, Return: analysis.returnObligation, Edge: analysis.edgeObligation,
		Successors: summaryKnowledge.Provider(analysis.pass).Successors(),
	})
	if outcome == ssaflow.ObligationHonored {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonJoinProven}
	}
	analysis.ruledOut(reasonJoinProven)
	if analysis.guardedLocalJoin() {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonGuardedLocalJoin}
	}
	analysis.ruledOut(reasonGuardedLocalJoin)
	if outcome == ssaflow.ObligationUncertain {
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
	return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: reasonUnownedReturn}
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
				closure, _ := common.Value.(*ssa.MakeClosure)
				if analysis.anyArgumentConsumes(common) || analysis.closureConsumes(closure) {
					return true
				}
			case *ssa.Call:
				for _, argument := range typed.Common().Args {
					if closure, ok := argument.(*ssa.MakeClosure); ok && analysis.closureConsumes(closure) {
						return true
					}
				}
			}
		}
	}
	return false
}

// lifecycleProof settles workers whose completion is owned outside the
// spawning function before any local flow is consulted.
func (analysis *spawnAnalysis) lifecycleProof() (GoroutineProof, bool) {
	if analysis.relayDependencyUncertain() {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonRelayDependency}, true
	}
	if analysis.config.mode == goroutineModeContext {
		if goroutineReceivesCallerSignal(analysis.pass, analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonStopLifecycle}, true
		}
		if goroutineReceivesCallerContext(analysis.pass, analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonContextLifecycle}, true
		}
		if goroutineReceivesLocallyCanceledContext(analysis.pass, analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonLocallyCanceledContext}, true
		}
		if goroutineReceivesReceiverContext(analysis.pass, analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonReceiverContext}, true
		}
	}
	// A goroutine that completes through a caller-owned channel or wait group
	// transfers its join obligation across the call boundary.
	for _, tracked := range analysis.tracked {
		if tracked.kind == trackedSignal && helperSignalOrigin(tracked.value, analysis.spawn, analysis.budget()) {
			return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}, true
		}
		if tracked.kind != trackedOwner && ssaflow.ExternallyOwnedValue(tracked.value) {
			return GoroutineProof{Outcome: GoroutineTransferred, Reason: reasonCallerOrExternalOwner}, true
		}
		if tracked.kind == trackedGroup && opaqueGroupOrigin(tracked.value, analysis.budget()) {
			return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}, true
		}
	}
	if synctestOwnsGoroutine(analysis.function) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonSynctestBubbleOwner}, true
	}
	return GoroutineProof{}, false
}

// A channel supplied by a factory or registry may already have another owner.
// Until its allocation and ownership are established, a local launch cannot
// manufacture an exclusive receive obligation for this caller. This is not a
// claim that an arbitrary factory channel is drained.
// https://github.com/deckarep/golang-set/blob/711c30df0fdf98710a4ca0211e12ef7210967ad3/threadsafe.go#L268-L287
func helperSignalOrigin(value ssa.Value, spawn *ssa.Go, budget *ssaflow.SearchBudget) bool {
	storage := ssaflow.NewStorage(budget)
	var leaf func(ssaflow.ReachingWalk, ssa.Value) bool
	leaf = func(walk ssaflow.ReachingWalk, current ssa.Value) bool {
		if resolved := storage.Resolve(current); resolved.Proven() && resolved.Value != current {
			return walk.Any(resolved.Value, leaf)
		}
		// A returned struct is copied into a local before invoking its pointer
		// method. That copy does not manufacture exclusive ownership of the
		// factory's channel; a companion result may be its actual join handle.
		// https://github.com/tus/tusd/blob/c9d174d0e20c69f24e9785d2f639df4da1c4fdc5/pkg/s3store/s3store_part_producer_test.go#L28-L56
		if _, allocated := current.(*ssa.Alloc); allocated {
			if content := storage.Content(current, spawn); content.Proven() && content.Value != current {
				return walk.Any(content.Value, leaf)
			}
		}
		if result, ok := current.(*ssa.Extract); ok {
			return walk.Any(result.Tuple, leaf)
		}
		_, call := current.(*ssa.Call)
		return call
	}
	return ssaflow.NewReachingWalk(carryForms).Any(value, leaf)
}

// An opaque producer can lend a registry-owned group, not allocate a new one.
// Without its body we cannot assign the join obligation to this invocation.
// This is uncertainty, not proof that the caller or registry actually waits.
// https://github.com/i-love-flamingo/flamingo/blob/79a55d62bb7a1bffe11a4dea1444490b14785879/core/requesttask/filter.go#L28-L60
func opaqueGroupOrigin(value ssa.Value, budget *ssaflow.SearchBudget) bool {
	storage := ssaflow.NewStorage(budget)
	var leaf func(ssaflow.ReachingWalk, ssa.Value) bool
	leaf = func(walk ssaflow.ReachingWalk, current ssa.Value) bool {
		if resolved := storage.Resolve(current); resolved.Proven() && resolved.Value != current {
			return walk.Any(resolved.Value, leaf)
		}
		switch typed := current.(type) {
		case *ssa.Extract:
			return walk.Any(typed.Tuple, leaf)
		case *ssa.Call:
			callee, _ := ssaflow.DirectCallee(typed.Common())
			return callee == nil || len(callee.Blocks) == 0
		}
		return false
	}
	return ssaflow.NewReachingWalk(carryForms|ssaflow.TransparentTypeAssert).Any(value, leaf)
}

// dominatingProof classifies the instructions that run before every spawn. A
// deferred join registered on every path to the spawn runs after the worker
// settles, as in Zap's pool race test:
// https://github.com/uber-go/zap/blob/bb1a55dd13257cf7cbd06b4146674c67ca614dea/internal/pool/pool_test.go#L85-L105
// A transfer or opaque handoff before the spawn likewise settles the
// obligation before this function could observe completion.
func (analysis *spawnAnalysis) dominatingProof() (GoroutineProof, bool) {
	unknown := false
	for _, block := range analysis.function.Blocks {
		if !block.Dominates(analysis.spawn.Block()) {
			continue
		}
		for _, instruction := range block.Instrs {
			if !ssaflow.InstructionDominates(instruction, analysis.spawn) || instruction == analysis.spawn {
				continue
			}
			_, deferred := instruction.(*ssa.Defer)
			// Testing callbacks also execute later, even when registration
			// precedes the spawn. An ordinary Wait before spawn still cannot join.
			// https://github.com/miniscruff/changie/blob/e78b7fcae4fd76fc588b6442117ef99c39835e15/then/write.go#L19-L35
			deferred = deferred || ssaflow.HasLibraryContract(ssaflow.InstructionCall(instruction), ssaflow.ContractTestingCleanup)
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
		honored := ssaflow.EvaluateObligation(ssaflow.ObligationFlow{
			Start: analysis.spawn, NonNil: created, Instruction: analysis.obligation, Return: analysis.returnObligation,
			Successors: summaryKnowledge.Provider(analysis.pass).Successors(),
		}) == ssaflow.ObligationHonored
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
	if !ok || ssaflow.BlockInCycle(analysis.spawn.Block()) {
		return nil
	}
	var created []ssa.Value
	for _, binding := range closure.Bindings {
		stored := ssaflow.NewStorage(analysis.budget()).StableContent(binding, analysis.spawn)
		channel, ok := stored.Value.(*ssa.MakeChan)
		if stored.Proven() && ok && channel.Parent() == analysis.function && !ssaflow.BlockInCycle(channel.Block()) {
			// The guard reads the captured cell after launch. StableContent
			// rules out replacement, but its earlier nil state must not become
			// a may-alias assumption in the shared non-nil flow query.
			for _, instruction := range ssaflow.InstructionsReachableAfter(analysis.spawn) {
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
