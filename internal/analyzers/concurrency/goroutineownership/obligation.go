package goroutineownership

import (
	"go/constant"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Obligation evidence identifies what a launched function promises its
// parent: a channel it sends on or closes, or a WaitGroup it settles.
// Lifecycle owners provide conservative acceptance evidence, never an
// obligation by themselves. Values are resolved back
// to the parent's SSA values at the spawn so later instructions can be matched
// exactly. A channel the worker only receives from bounds the worker instead
// and is handled as lifecycle evidence, never as a join obligation.

type trackedKind uint8

const (
	// trackedSignal is a channel the worker sends on or closes; the parent
	// joins by receiving from it.
	trackedSignal trackedKind = iota
	// trackedGroup is a WaitGroup whose Done settles the worker; the parent
	// joins by waiting on it.
	trackedGroup
	// trackedOwner is a captured value with a lifecycle method. It can suppress
	// a warning when settled, but cannot establish a completion obligation.
	trackedOwner
)

type trackedValue struct {
	value ssa.Value
	kind  trackedKind
}

type joinEdgeEvidence struct {
	reason goroutineOwnershipReason
	action ssaflow.ObligationAction
}

func resolveSpawnedFunction(pass *analysis.Pass, spawn *ssa.Go, budget *ssaflow.SearchBudget) (*ssa.Function, *ssa.MakeClosure) {
	function, closure := ssaflow.DirectCallee(spawn.Common())
	if closure != nil {
		return function, closure
	}
	// A helper that invokes a zero-argument callback before every normal
	// return is transparent to the spawned worker's lifecycle. Panic-reporting
	// and tracing wrappers commonly use this shape. Analyze the callback body
	// so its context, completion signal, and lifecycle owner remain visible.
	evidence, _ := summaryKnowledge.Provider(pass).LifecycleEvidence("goroutineownership", string(check.GoroutineJoin))
	for index, argument := range spawn.Common().Args {
		if !budget.Spend() {
			return nil, nil
		}
		callback, callbackClosure := callbackTarget(argument)
		if callback == nil || len(callback.Params) != 0 {
			continue
		}
		invoked := lifecycle.ProveSpawnedInvocation(spawn, argument, budget).Proven()
		if !invoked {
			invoked, _ = evidence.CalleeClaims(spawn, index, lifecyclefacts.ClaimSynchronouslyInvokes)
		}
		if !invoked {
			continue
		}
		return ssaflow.ResolvedFunction(callback), callbackClosure
	}
	return function, closure
}

func callbackTarget(value ssa.Value) (*ssa.Function, *ssa.MakeClosure) {
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok && inner != value {
		return callbackTarget(inner)
	}
	switch typed := value.(type) {
	case *ssa.Function:
		return typed, nil
	case *ssa.MakeClosure:
		function, _ := typed.Fn.(*ssa.Function)
		return function, typed
	default:
		return nil, nil
	}
}

func spawnedCompletionValues(
	pass *analysis.Pass,
	spawn *ssa.Go,
	budget *ssaflow.SearchBudget,
) (signals, groups []ssa.Value, unsettledDone ssa.Instruction) {
	function, closure := resolveSpawnedFunction(pass, spawn, budget)
	if function == nil {
		return nil, nil, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return signals, nil, nil
			}
			// A disabled optional channel cannot establish a join obligation at
			// this call site, even if another invocation uses the worker's send.
			// https://github.com/paradigmxyz/iron-proxy/blob/5bd11abeb95ca734c767cfc992ea9be862700614/internal/postgres/manager.go#L64-L97
			if signal := spawnedCompletionSignal(spawn, function, closure, instruction, budget); signal != nil && !ssaflow.DefinitelyNil(signal) {
				signals = append(signals, signal)
			}
		}
	}
	groups, unsettledDone = waitGroupCompletionValues(spawn, function, closure, budget)
	groups = append(groups, deferredCompletionGroups(spawn, function, closure, budget)...)
	return signals, groups, unsettledDone
}

// A deferred helper can finish a group after releasing other worker resources.
// Keep that group as an alternative completion handle, so returning its owner
// is visible even when the worker also sends on an unrelated output channel.
// https://github.com/vbauerster/mpb/blob/ddeb4bb7bcb86e114648760018b10700841a081a/heap_manager.go#L46-L61
func deferredCompletionGroups(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, budget *ssaflow.SearchBudget) []ssa.Value {
	var groups []ssa.Value
	for _, pair := range ssaflow.CallBindings(spawn.Common(), function, closure) {
		if !budget.Spend() {
			return groups
		}
		group := completionValueAtCall(spawn, function, closure, pair.Local, budget)
		// A typed nil actual satisfies the parameter's static WaitGroup type,
		// but the callee's guarded deferred Done cannot run for this launch.
		if group == nil || ssaflow.DefinitelyNil(group) || !syntax.NamedType(group.Type(), "sync", "WaitGroup") {
			continue
		}
		// A conditional registration promises completion only on that branch.
		// It cannot create an unconditional obligation for the parent, which may
		// use the same flag to decide whether to wait. Require return coverage,
		// not merely one deferred helper that would call Done if registered.
		// https://github.com/hashicorp/vault-secrets-operator/blob/451a61fc0eda5b26e65dedb03e76fa8ec02b2984/vault/client_factory.go#L890-L909
		unsettled := completionReturnCoverage(function, pair.Local, budget, func(instruction ssa.Instruction) bool {
			deferred, ok := instruction.(*ssa.Defer)
			if !ok {
				return false
			}
			if ssaflow.CallMatchesSymbol(deferred.Common(), waitGroupDone) &&
				heapmodel.DefinitelySameValue(ssaflow.CallReceiver(deferred.Common()), pair.Local) {
				return true
			}
			proof := lifecycle.ProveCompletion(lifecycle.CompletionRequest{
				Instruction: deferred, Target: pair.Local, Methods: []string{"Done"},
				Budget: budget,
			})
			return proof.Proven()
		}) != ssaflow.ObligationHonored

		if !unsettled {
			groups = append(groups, group)
		}
	}
	return groups
}

// spawnedCompletionSignal resolves a send or close performed by the worker, or
// by a closure the worker calls synchronously, back to the parent's channel.
func spawnedCompletionSignal(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	instruction ssa.Instruction,
	budget *ssaflow.SearchBudget,
) ssa.Value { //nolint:ireturn // Completion signals retain their concrete SSA value types.
	if channel := completionNotification(instruction, budget); channel != nil {
		if _, send := instruction.(*ssa.Send); !send && !notifiesChannelOnEveryReturn(function, channel, budget) {
			return nil
		}
		return signalSuppliedAtCall(spawn, function, closure, channel, budget)
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return nil
	}
	if _, launched := instruction.(*ssa.Go); launched {
		return nil
	}
	nested, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return nil
	}
	// The inner notification and its invocation must both cover returns.
	// A conditional defer cannot promise an unconditional worker join.
	covered := completionReturnCoverage(function, nil, budget, func(candidate ssa.Instruction) bool {
		return candidate == instruction
	}) == ssaflow.ObligationHonored
	_, deferred := instruction.(*ssa.Defer)
	if !covered || (!deferred && !terminalCompletion(instruction, budget)) {
		return nil
	}
	if signal := nestedClosureSignal(nested, budget); signal != nil {
		return signalSuppliedAtCall(spawn, function, closure, signal, budget)
	}
	return nil
}

// completionNotification selects the same completion operations for direct
// discovery, nested closures and return coverage. A detached notification
// belongs to another worker and cannot settle this one.
func completionNotification(
	instruction ssa.Instruction, budget *ssaflow.SearchBudget,
) ssa.Value { //nolint:ireturn // Notifications retain their concrete SSA value types.
	if _, launched := instruction.(*ssa.Go); launched {
		return nil
	}
	if send, ok := instruction.(*ssa.Send); ok {
		// A send followed by further work can announce readiness or progress,
		// not completion. Blocking-producer checks use the full send lifecycle.
		// https://github.com/kubernetes-sigs/cluster-proportional-autoscaler/blob/39dd2288da294e98d683c5619fc3556016df1e76/pkg/autoscaler/autoscaler_server.go#L109-L124
		if terminalCompletion(send, budget) {
			return send.Chan
		}
		return nil
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil || !ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) || len(common.Args) != 1 {
		return nil
	}
	// Closing entries in a loop is per-item cleanup. A deferred close can
	// announce completion, provided its registration covers every return.
	// https://github.com/BurntSushi/wingo/blob/33b154361587e65ec35d4499f1cc487835d0ab48/event/ipc.go#L124-L157
	if _, deferred := instruction.(*ssa.Defer); !deferred && ssaflow.BlockInCycle(instruction.Block()) {
		return nil
	}
	return common.Args[0]
}

// An error-only notification is not a promise to signal every worker exit.
// Require exact notification coverage before treating the channel as completion;
// otherwise the caller may legitimately observe a separate success event.
// https://github.com/zmap/zgrab2/blob/a1231792c51576f1818825fae51db042b4dcd41e/lib/http2/transport.go#L3028-L3043
func notifiesChannelOnEveryReturn(function *ssa.Function, channel ssa.Value, budget *ssaflow.SearchBudget) bool {
	if !completionHasReturn(function, budget) {
		return false
	}
	identity := channel
	if source, ok := ssaflow.IdentitySource(channel); ok {
		identity = source
	}
	return completionReturnCoverage(function, channel, budget, func(instruction ssa.Instruction) bool {
		notified := completionNotification(instruction, budget)
		if source, ok := ssaflow.IdentitySource(notified); ok {
			notified = source
		}
		return heapmodel.DefinitelySameValue(notified, identity)
	}) == ssaflow.ObligationHonored
}

// nestedClosureSignal returns the worker-level value that a synchronously
// invoked inner closure sends on or closes. A deferred inner closure is the
// common shape: `defer func() { done <- recover() }()`.
func nestedClosureSignal(
	nested *ssa.MakeClosure, budget *ssaflow.SearchBudget,
) ssa.Value { //nolint:ireturn // Join handles retain their concrete SSA value types.
	function, _ := nested.Fn.(*ssa.Function)
	if function == nil {
		return nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return nil
			}
			channel := completionNotification(instruction, budget)
			if channel == nil || !notifiesChannelOnEveryReturn(function, channel, budget) {
				continue
			}
			storage := heapmodel.NewStorage(budget)
			source := channel
			if identity, ok := ssaflow.IdentitySource(channel); ok {
				source = identity
			}
			for _, captured := range ssaflow.ClosureBindingPairs(function, nested) {
				if storage.Same(source, captured.Free).Proven() &&
					ssaflow.CallbackCaptureReadOnly(nested, captured.Binding, budget) {
					return captured.Binding
				}
			}
		}
	}
	return nil
}

var waitGroupDone = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Done"})

var waitGroupWait = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Wait"})

// waitGroupCompletionValues distinguishes a worker's settlement from an earlier
// progress signal. A direct Done must be terminal on every normal path and a
// deferred Done must be registered on every normal path; otherwise Wait can
// return while the worker still runs, as in Moov's test:
// https://github.com/moov-io/rtp20022/blob/0b08f38d0a1341d61a4d1fe7b0a402b5718d3f30/pkg/rtp/restrictions_test.go#L27-L43
func waitGroupCompletionValues(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	budget *ssaflow.SearchBudget,
) (groups []ssa.Value, unsettled ssa.Instruction) {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return groups, unsettled
			}
			if _, launched := instruction.(*ssa.Go); launched {
				// `go group.Done()` is a readiness notification detached from the
				// worker's own completion, not a join obligation for that worker.
				// https://github.com/pancsta/asyncmachine-go/blob/cce9b31145cb07c1262ac0c71a696222b0119b75/examples/subscriptions/example_subscriptions.go#L34-L79
				continue
			}
			common := ssaflow.InstructionCall(instruction)
			if common == nil || !ssaflow.CallMatchesSymbol(common, waitGroupDone) {
				continue
			}
			receiver := ssaflow.CallReceiver(common)
			group := completionValueAtCall(spawn, function, closure, receiver, budget)
			// A callee's nil-guarded Done is not a promise when this launch passes
			// nil. OpenIM's fire-and-forget branch uses the same worker as its
			// counted branch but supplies a nil group:
			// https://github.com/openimsdk/openim-sdk-core/blob/061ac673ffa31f4d863651fdffee7882609a5f62/internal/conversation_msg/notification.go#L441-L469
			if group == nil || ssaflow.DefinitelyNil(group) || heapmodel.MayAliasAny(group, groups) {
				continue
			}
			if !waitGroupSettlesFunction(function, receiver, budget) {
				// A deferred Done cannot be an early progress notification. If its
				// registration is conditional, the completion promise is unknown;
				// do not misclassify missing coverage as Done preceding work.
				if _, deferred := instruction.(*ssa.Defer); deferred {
					continue
				}
				// Repeated Done calls can count completed items rather than workers.
				// Without counter arithmetic a backedge is not proof of early Done.
				// https://github.com/grafana/dskit/blob/86f3c54f61fe477e68ac15dfac9ed88e4ee9e457/ring/batch_test.go#L61-L74
				if ssaflow.BlockInCycle(instruction.Block()) {
					continue
				}
				if unsettled == nil {
					unsettled = instruction
				}
				continue
			}
			groups = append(groups, group)
		}
	}
	return groups, unsettled
}

func waitGroupSettlesFunction(function *ssa.Function, receiver ssa.Value, budget *ssaflow.SearchBudget) bool {
	if !completionHasReturn(function, budget) {
		return false
	}
	// Paths reachable only when the group is nil carry no obligation: without a
	// WaitGroup nothing was added and nothing waits, so a Done guarded by a nil
	// check on the group itself still settles every path that has a group.
	// Vitess passes a group only on the shutdown path that waits for it:
	// https://github.com/vitessio/vitess/blob/44321d8ca0e2b2689e869bc680b6ce6402bba977/go/vt/vttablet/tabletserver/state_manager.go#L605-L631
	return completionReturnCoverage(function, receiver, budget, func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		if common == nil || !ssaflow.CallMatchesSymbol(common, waitGroupDone) ||
			!ssaflow.MayAliasThroughLoads(ssaflow.CallReceiver(common), receiver) {
			return false
		}
		if _, deferred := instruction.(*ssa.Defer); deferred {
			return true
		}
		return terminalCompletion(instruction, budget)
	}) == ssaflow.ObligationHonored
}

// terminalCompletion reports whether only returns can follow a completion
// operation. Later work cannot be joined by observing an earlier signal.
func terminalCompletion(done ssa.Instruction, budget *ssaflow.SearchBudget) bool {
	index := ssaflow.InstructionIndex(done)
	if index < 0 {
		return false
	}
	type cursor struct {
		block *ssa.BasicBlock
		index int
	}
	queue := []cursor{{block: done.Block(), index: index + 1}}
	seen := make(map[cursor]bool)
	for len(queue) > 0 {
		if !budget.Spend() {
			return false
		}
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			return false
		}
		seen[current] = true
		if current.index < len(current.block.Instrs) {
			switch current.block.Instrs[current.index].(type) {
			case *ssa.Return:
				continue
			case *ssa.RunDefers:
				if !ssaflow.CallMatchesSymbol(ssaflow.InstructionCall(done), waitGroupDone) || !completionOnlyDefersWithin(done.Parent(), budget) {
					return false
				}
				queue = append(queue, cursor{block: current.block, index: current.index + 1})
				continue
			case *ssa.Jump:
				// Conditional terminal sends can jump to a shared return block.
				// A jump performs no work and preserves this completion proof.
			default:
				return false
			}
		}
		if len(current.block.Succs) == 0 {
			return false
		}
		for _, successor := range current.block.Succs {
			queue = append(queue, cursor{block: successor})
		}
	}
	return true
}

// A terminal Done followed only by other Done/close defers has finished the
// worker's actual work. Keeping that alternative handle does not treat an
// arbitrary deferred callback as complete: it may still block or mutate data.
// https://github.com/murphysecurity/murphysec/blob/59d5cdc9a53a9e7940250aa30ea4434d0e258c40/module/nuget/nuget_cmd_build.go#L611-L640
func completionOnlyDefersWithin(function *ssa.Function, budget *ssaflow.SearchBudget) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			deferred, ok := instruction.(*ssa.Defer)
			if !ok {
				continue
			}
			common := deferred.Common()
			if !ssaflow.CallMatchesSymbol(common, waitGroupDone) &&
				!ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) {
				return false
			}
		}
	}
	return true
}

// sharedStorageSignals reports whether every completion signal was reached
// through an element of an aggregate. Such a signal belongs to whatever else
// holds that aggregate, such as a session the caller already stored in its
// registry, so an unjoined launch is not proven to be this function's
// obligation. MikroDash starts sessions ranged from a slice it filled while
// registering them:
// https://github.com/SecOps-7/MikroDash/blob/fb859d40ea22a2e46fb5a51d5b4ee7940bce6c9a/internal/alertpool/pool.go#L200-L223
func (analysis *spawnAnalysis) sharedStorageSignals() bool {
	if len(analysis.signals) == 0 || len(analysis.groups) > 0 {
		return false
	}
	return !slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
		return !ssaflow.ElementOfAggregate(signal)
	})
}

// bufferedSignals reports whether every completion signal is a locally created
// channel with a non-zero buffer.
func (analysis *spawnAnalysis) bufferedSignals() bool {
	if len(analysis.signals) == 0 || len(analysis.groups) > 0 {
		return false
	}
	return !slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
		return !bufferedLocalChannel(analysis.function, signal)
	})
}

// localChannel returns the channel made in function that reaches signal.
func localChannel(function *ssa.Function, signal ssa.Value) *ssa.MakeChan {
	return localChannelWithin(function, signal, nil)
}

func localChannelWithin(function *ssa.Function, signal ssa.Value, budget *ssaflow.SearchBudget) *ssa.MakeChan {
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		created, ok := instruction.(*ssa.MakeChan)
		if !ok {
			continue
		}
		if carries(ssaflow.NewReachingWalk(carryForms).Within(budget), signal, created) {
			return created
		}
	}
	return nil
}

func bufferedLocalChannel(function *ssa.Function, signal ssa.Value) bool {
	created := localChannel(function, signal)
	if created == nil {
		return false
	}
	size, constantSize := created.Size.(*ssa.Const)
	return !constantSize || size.Value == nil || constant.Sign(size.Value) > 0
}

// completionHasReturn requires a witness before coverage can create a promise.
func completionHasReturn(function *ssa.Function, budget *ssaflow.SearchBudget) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			if _, ok := instruction.(*ssa.Return); ok {
				return true
			}
		}
	}
	return false
}

// completionReturnCoverage charges both examined instructions and flow states.
// The supplied predicate owns operation policy; exhaustion remains uncertain.
func completionReturnCoverage(
	function *ssa.Function, nonNil ssa.Value, budget *ssaflow.SearchBudget, owns func(ssa.Instruction) bool,
) ssaflow.ObligationOutcome {
	return ssaflow.EvaluateObligationFromEntry(function, ssaflow.ObligationFlow{
		Budget: budget, NonNil: nonNil,
		Instruction: func(instruction ssa.Instruction) ssaflow.ObligationAction {
			if !budget.Spend() {
				return ssaflow.ObligationUnknown
			}
			if owns(instruction) {
				return ssaflow.ObligationExact
			}
			return ssaflow.ObligationNone
		},
	})
}
