package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// This file owns the single decision path for every goroutineownership check.
// The proof reverses the burden of evidence: the worker must first establish an
// obligation (a completion signal, a settling WaitGroup, or, for the detached
// audit, a lifecycle owner), and the default diagnostic then needs a feasible
// return path on which nothing joins, transfers, or ambiguously consumes it.
// Every instruction after the spawn is classified once; the flow query asks
// only whether an exact action, or any action at all, covers every return.

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
	reasonSynctestBubbleOwner     goroutineOwnershipReason = "synctest-bubble-owner"
	reasonCallerOrExternalOwner   goroutineOwnershipReason = "caller-or-external-owner"
	reasonOwnershipTransfer       goroutineOwnershipReason = "ownership-transfer"
	reasonOpaqueTransfer          goroutineOwnershipReason = "opaque-ownership-transfer"
	reasonLoopJoinUnproven        goroutineOwnershipReason = "loop-join-unproven"
	reasonWorkerConsumesSignal    goroutineOwnershipReason = "signal-consumed-by-worker"
	reasonFlagGuardedJoin         goroutineOwnershipReason = "flag-guarded-join"
	reasonBufferedSignal          goroutineOwnershipReason = "buffered-completion-signal"
	reasonDetachedUnknown         goroutineOwnershipReason = "detached-lifecycle-unknown"
	reasonUnownedReturn           goroutineOwnershipReason = "unowned-return"
	reasonDoneBeforeCompletion    goroutineOwnershipReason = "waitgroup-done-before-completion"
)

// GoroutineProof is the single result consumed by reporting, tracing, and
// cross-analyzer ownership queries.
type GoroutineProof struct {
	Outcome GoroutineOutcome
	Reason  goroutineOwnershipReason
}

// GoroutineOwnershipMayBeHandledInTest conservatively reports whether an exact
// proof or an opaque handoff may own spawn independently of context
// cancellation. Unknown handoffs suppress testlifecycle: they are not positive
// ownership evidence, but neither analyzer can prove a detached test worker.
func GoroutineOwnershipMayBeHandledInTest(spawn *ssa.Go) bool {
	if spawn == nil || spawn.Parent() == nil {
		return false
	}
	proof := newSpawnAnalysis(spawn.Parent(), spawn, goroutineOwnershipConfig{mode: goroutineModeContext}).prove()
	return proof.Outcome == GoroutineLifecycleHonored || proof.Outcome == GoroutineTransferred ||
		proof.Outcome == GoroutineUnknown && proof.Reason != reasonDetachedUnknown
}

func (analysis *spawnAnalysis) prove() GoroutineProof {
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
	exact := func(instruction ssa.Instruction) bool {
		action := analysis.action(instruction)
		return action == actionJoin || action == actionTransfer
	}
	if !ssaflow.UnownedReturn(analysis.spawn, exact, analysis.returnTransfers) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonJoinProven}
	}
	if analysis.guardedLocalJoin(exact) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonGuardedLocalJoin}
	}
	any := func(instruction ssa.Instruction) bool {
		return analysis.action(instruction) != actionNone
	}
	if !ssaflow.UnownedReturn(analysis.spawn, any, analysis.returnTransfers) {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}
	}
	if analysis.flagGuardedJoin() {
		// A join guarded by a local Boolean that the function assigns around
		// the spawn, such as `started = true` before launching and `if started
		// { wg.Wait() }` after, correlates the guard with the launch in a way
		// this proof does not model. Soperator and gocoin both use the shape:
		// https://github.com/nebius/soperator/blob/3f5635c08fab3578db574a55b293b5aa32042bd3/internal/exporter/exporter.go#L157-L193
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonFlagGuardedJoin}
	}
	if analysis.countedJoin() {
		// When the spawn itself runs in a loop, a later receive loop that may
		// run zero times is not a proven skip: the spawn loop may have run zero
		// times too. Matching those counts is not modeled, so counted joins stay
		// unknown. A single spawn joined only inside a conditional loop body has
		// no such symmetry and remains reportable.
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonLoopJoinUnproven}
	}
	if analysis.checkID == check.GoroutineDetached {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonDetachedUnknown}
	}
	if analysis.bufferedSignals() {
		// A buffered completion send lets the worker finish after the caller
		// stops receiving, so it does not by itself establish a join protocol.
		// Buildkite uses a one-slot result channel to let its collector finish:
		// https://github.com/buildkite/agent/blob/e206ddf806af50a1ba8c9a6dd501dfda0b730818/internal/artifact/downloader.go#L96-L177
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonBufferedSignal}
	}
	if analysis.unsettledDone != nil && len(analysis.groups) == 0 && len(analysis.signals) == 0 {
		return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: reasonDoneBeforeCompletion}
	}
	return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: reasonUnownedReturn}
}

// otherWorkerConsumesSignal reports whether a goroutine other than the spawn,
// launched anywhere in the function, captures or receives a tracked signal.
func (analysis *spawnAnalysis) otherWorkerConsumesSignal() bool {
	if len(analysis.signals) == 0 {
		return false
	}
	for _, block := range analysis.function.Blocks {
		for _, instruction := range block.Instrs {
			launched, ok := instruction.(*ssa.Go)
			if !ok || launched == analysis.spawn {
				continue
			}
			common := launched.Common()
			closure, _ := common.Value.(*ssa.MakeClosure)
			if analysis.anyArgumentConsumes(common) || analysis.closureConsumes(closure) {
				return true
			}
		}
	}
	return false
}

// flagGuardedJoin reports whether a join reachable from the spawn sits under a
// branch on a local Boolean flag that receives a constant on a path through
// the spawn. The assignment may run before or after the launch; what matters
// is that the guard's value is decided by the same control flow that decided
// to launch.
func (analysis *spawnAnalysis) flagGuardedJoin() bool {
	for _, block := range analysis.function.Blocks {
		for _, instruction := range block.Instrs {
			if deferred, ok := instruction.(*ssa.Defer); ok && analysis.deferredClosureFlagGuardsJoin(deferred) {
				return true
			}
			branch, ok := instruction.(*ssa.If)
			if !ok || !ssaflow.InstructionMayFollow(analysis.spawn, branch) || !analysis.branchGuardsJoin(branch) {
				continue
			}
			if flag := booleanFlagVariable(branch.Cond); flag != nil && analysis.flagAssignedAroundSpawn(flag) {
				return true
			}
		}
	}
	return false
}

// deferredClosureFlagGuardsJoin recognizes the same correlation inside a
// deferred closure: `defer func() { if wait4compl { wg.Wait() } }()` with
// wait4compl assigned beside the launch. gocoin verifies transaction scripts
// this way:
// https://github.com/piotrnar/gocoin/blob/467131d21dd0c9252f99c00d99ba9ca74f60ca1e/lib/chain/chain_accept.go#L125-L133
func (analysis *spawnAnalysis) deferredClosureFlagGuardsJoin(deferred *ssa.Defer) bool {
	callee, closure := calledFunction(deferred.Common())
	if closure == nil || callee == nil {
		return false
	}
	pairs := suppliedValues(deferred.Common(), callee, closure)
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			branch, ok := instruction.(*ssa.If)
			if !ok {
				continue
			}
			flag := capturedFlagVariable(branch.Cond, pairs)
			if flag == nil || !analysis.flagAssignedAroundSpawn(flag) {
				continue
			}
			for _, successor := range branch.Block().Succs {
				if analysis.closureBlockJoins(successor, pairs) {
					return true
				}
			}
		}
	}
	return false
}

// capturedFlagVariable maps a branch on a loaded free variable back to the
// Boolean local it was bound to.
func capturedFlagVariable(condition ssa.Value, pairs []suppliedValue) ssa.Value { //nolint:ireturn // Flags are allocations.
	if negation, ok := condition.(*ssa.UnOp); ok && negation.Op == token.NOT {
		condition = negation.X
	}
	load, ok := condition.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	for _, pair := range pairs {
		if pair.local == load.X {
			if flag, ok := pair.supplied.(*ssa.Alloc); ok && isBooleanPointer(flag.Type()) {
				return flag
			}
		}
	}
	return nil
}

// closureBlockJoins reports whether block joins a tracked value through one of
// the closure's captured variables.
func (analysis *spawnAnalysis) closureBlockJoins(block *ssa.BasicBlock, pairs []suppliedValue) bool {
	for _, pair := range pairs {
		for _, tracked := range analysis.tracked {
			if !bindingCarries(pair.supplied, tracked.value) {
				continue
			}
			derives := func(value ssa.Value) bool {
				return ssaflow.ValueDerivesFrom(value, pair.local, map[ssa.Value]bool{})
			}
			for _, instruction := range block.Instrs {
				if instructionJoins(instruction, tracked.kind, derives, map[*ssa.Function]bool{}) {
					return true
				}
			}
		}
	}
	return false
}

// branchGuardsJoin reports whether a successor of branch contains a join,
// transfer, or opaque handoff of a tracked value that the other successor can
// skip. A launched waiter counts: the guard decides whether the parent hands
// the group to it.
func (analysis *spawnAnalysis) branchGuardsJoin(branch *ssa.If) bool {
	for _, successor := range branch.Block().Succs {
		for _, instruction := range successor.Instrs {
			if analysis.action(instruction) != actionNone {
				return true
			}
		}
	}
	return false
}

// booleanFlagVariable returns the Boolean flag a branch condition reads,
// following one negation. A flag that no closure captures is an SSA phi of
// constants; a captured one is an addressable local.
func booleanFlagVariable(condition ssa.Value) ssa.Value { //nolint:ireturn // Flags are phis or allocations.
	if negation, ok := condition.(*ssa.UnOp); ok && negation.Op == token.NOT {
		condition = negation.X
	}
	if phi, ok := condition.(*ssa.Phi); ok && phiCarriesBooleanConstant(phi, map[*ssa.Phi]bool{}) {
		return phi
	}
	load, ok := condition.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	if flag, ok := load.X.(*ssa.Alloc); ok && isBooleanPointer(flag.Type()) {
		return flag
	}
	return nil
}

// phiCarriesBooleanConstant reports whether a Boolean constant reaches phi,
// possibly through the nested phis a loop with continue statements creates.
func phiCarriesBooleanConstant(phi *ssa.Phi, seen map[*ssa.Phi]bool) bool {
	if seen[phi] {
		return false
	}
	seen[phi] = true
	return slices.ContainsFunc(phi.Edges, func(edge ssa.Value) bool {
		if inner, ok := edge.(*ssa.Phi); ok {
			return phiCarriesBooleanConstant(inner, seen)
		}
		return isBooleanConstant(edge)
	})
}

func isBooleanConstant(value ssa.Value) bool {
	literal, ok := value.(*ssa.Const)
	if !ok || literal.Value == nil {
		return false
	}
	basic, ok := literal.Type().Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func isBooleanPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	basic, ok := pointer.Elem().Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

// flagAssignedAroundSpawn reports whether a constant reaches the flag from a
// block that the spawn dominates, is dominated by, or shares.
func (analysis *spawnAnalysis) flagAssignedAroundSpawn(flag ssa.Value) bool {
	if phi, ok := flag.(*ssa.Phi); ok {
		return analysis.phiConstantAroundSpawn(phi, map[*ssa.Phi]bool{})
	}
	if flag.Referrers() == nil {
		return false
	}
	for _, reference := range *flag.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == flag && isBooleanConstant(store.Val) && analysis.blockAroundSpawn(store.Block()) {
			return true
		}
	}
	return false
}

func (analysis *spawnAnalysis) phiConstantAroundSpawn(phi *ssa.Phi, seen map[*ssa.Phi]bool) bool {
	if seen[phi] {
		return false
	}
	seen[phi] = true
	for index, edge := range phi.Edges {
		if index >= len(phi.Block().Preds) {
			break
		}
		if inner, ok := edge.(*ssa.Phi); ok && analysis.phiConstantAroundSpawn(inner, seen) {
			return true
		}
		if isBooleanConstant(edge) && analysis.blockAroundSpawn(phi.Block().Preds[index]) {
			return true
		}
	}
	return false
}

func (analysis *spawnAnalysis) blockAroundSpawn(block *ssa.BasicBlock) bool {
	spawnBlock := analysis.spawn.Block()
	return block == spawnBlock || block.Dominates(spawnBlock) || spawnBlock.Dominates(block)
}

func (analysis *spawnAnalysis) countedJoin() bool {
	if !ssaflow.BlockInCycle(analysis.spawn.Block()) {
		return false
	}
	for _, block := range analysis.function.Blocks {
		if !ssaflow.BlockInCycle(block) {
			continue
		}
		for _, instruction := range block.Instrs {
			if ssaflow.InstructionMayFollow(analysis.spawn, instruction) && analysis.action(instruction) == actionJoin {
				return true
			}
		}
	}
	return false
}

// lifecycleProof settles workers whose completion is owned outside the
// spawning function before any local flow is consulted.
func (analysis *spawnAnalysis) lifecycleProof() (GoroutineProof, bool) {
	if analysis.config.mode == goroutineModeContext {
		if goroutineReceivesCallerSignal(analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonStopLifecycle}, true
		}
		if goroutineReceivesCallerContext(analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonContextLifecycle}, true
		}
	}
	// A goroutine that completes through a caller-owned channel or wait group
	// transfers its join obligation across the call boundary.
	for _, tracked := range analysis.tracked {
		if tracked.kind != trackedOwner && ssaflow.ExternallyOwnedValue(tracked.value) {
			return GoroutineProof{Outcome: GoroutineTransferred, Reason: reasonCallerOrExternalOwner}, true
		}
	}
	if synctestOwnsGoroutine(analysis.function) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonSynctestBubbleOwner}, true
	}
	return GoroutineProof{}, false
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
func (analysis *spawnAnalysis) guardedLocalJoin(exact func(ssa.Instruction) bool) bool {
	if len(analysis.signals)+len(analysis.groups) == 0 {
		return false
	}
	for _, created := range analysis.channelsCreatedOnceBeforeSpawn() {
		if !ssaflow.UnownedReturnAssumingNonNil(analysis.spawn, created, exact, analysis.returnTransfers) {
			return true
		}
	}
	return false
}

// channelsCreatedOnceBeforeSpawn returns captured channel locals whose only
// store is one MakeChan that dominates the spawn outside any loop. Any other
// use of the captured address, or a store that can execute repeatedly, means
// the guard may observe a different channel instance than the worker.
func (analysis *spawnAnalysis) channelsCreatedOnceBeforeSpawn() []ssa.Value {
	closure, ok := analysis.spawn.Common().Value.(*ssa.MakeClosure)
	if !ok || ssaflow.BlockInCycle(analysis.spawn.Block()) {
		return nil
	}
	var created []ssa.Value
	for _, binding := range closure.Bindings {
		if channel := singleDominatingChannelStore(analysis.function, analysis.spawn, closure, binding); channel != nil {
			created = append(created, channel)
		}
	}
	return created
}

func singleDominatingChannelStore(
	parent *ssa.Function,
	spawn *ssa.Go,
	closure *ssa.MakeClosure,
	binding ssa.Value,
) ssa.Value { //nolint:ireturn // Preserve the concrete channel value.
	if binding == nil || binding.Referrers() == nil {
		return nil
	}
	var stored ssa.Value
	for _, reference := range *binding.Referrers() {
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.UnOp:
			if typed.Op == token.MUL && typed.X == binding {
				continue
			}
		case *ssa.MakeClosure:
			if typed == closure {
				continue
			}
		case *ssa.Store:
			channel, ok := typed.Val.(*ssa.MakeChan)
			if ok && typed.Addr == binding && channel.Parent() == parent && stored == nil &&
				ssaflow.InstructionDominates(typed, spawn) && !ssaflow.BlockInCycle(typed.Block()) {
				stored = channel
				continue
			}
		}
		return nil
	}
	return stored
}
