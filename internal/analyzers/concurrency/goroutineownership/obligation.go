package goroutineownership

import (
	"go/constant"
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

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

type spawnAnalysis struct {
	pass          *analysis.Pass
	function      *ssa.Function
	spawn         *ssa.Go
	config        goroutineOwnershipConfig
	checkID       check.ID
	signals       []ssa.Value
	groups        []ssa.Value
	owners        []ssa.Value
	tracked       []trackedValue
	unsettledDone ssa.Instruction
	actions       map[ssa.Instruction]ownershipAction
	// tracing gates the record of ruled-out steps, which is worth keeping only
	// when a reader will see it.
	tracing    bool
	considered []goroutineOwnershipReason
}

func newSpawnAnalysis(
	pass *analysis.Pass,
	function *ssa.Function,
	spawn *ssa.Go,
	config goroutineOwnershipConfig,
) *spawnAnalysis {
	analysis := &spawnAnalysis{
		pass:     pass,
		function: function,
		spawn:    spawn,
		config:   config,
		actions:  make(map[ssa.Instruction]ownershipAction),
	}
	analysis.signals, analysis.groups, analysis.unsettledDone = spawnedCompletionValues(pass, spawn)
	if config.mode != goroutineModeJoin {
		analysis.owners = spawnedLifecycleOwners(pass, spawn)
	}
	for _, signal := range analysis.signals {
		analysis.tracked = append(analysis.tracked, trackedValue{value: signal, kind: trackedSignal})
	}
	for _, group := range analysis.groups {
		analysis.tracked = append(analysis.tracked, trackedValue{value: group, kind: trackedGroup})
	}
	for _, owner := range analysis.owners {
		analysis.tracked = append(analysis.tracked, trackedValue{value: owner, kind: trackedOwner})
	}
	analysis.checkID = check.GoroutineJoin
	analysis.tracing = analysisTrace.Enabled("goroutineownership", string(analysis.checkID))
	return analysis
}

func spawnedFunction(pass *analysis.Pass, spawn *ssa.Go) (*ssa.Function, *ssa.MakeClosure) {
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
		return ssaflow.ResolvedFunction(function), closure
	}
	// A helper that invokes a zero-argument callback before every normal
	// return is transparent to the spawned worker's lifecycle. Panic-reporting
	// and tracing wrappers commonly use this shape. Analyze the callback body
	// so its context, completion signal, and lifecycle owner remain visible.
	evidence := lifecyclefacts.NewLifecycleEvidence(pass, "goroutineownership", string(check.GoroutineJoin))
	for index, argument := range spawn.Common().Args {
		callback, callbackClosure := callbackTarget(argument)
		if callback == nil || len(callback.Params) != 0 {
			continue
		}
		invoked := ssaflow.SpawnInvokesArgumentOnEveryReturn(spawn, argument)
		if !invoked {
			invoked, _ = evidence.CalleeClaims(spawn, index, lifecyclefacts.ClaimSynchronouslyInvokes)
		}
		if !invoked {
			continue
		}
		return ssaflow.ResolvedFunction(callback), callbackClosure
	}
	return ssaflow.ResolvedFunction(function), closure
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
) (signals, groups []ssa.Value, unsettledDone ssa.Instruction) {
	function, closure := spawnedFunction(pass, spawn)
	if function == nil {
		return nil, nil, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			// A disabled optional channel cannot establish a join obligation at
			// this call site, even if another invocation uses the worker's send.
			// https://github.com/paradigmxyz/iron-proxy/blob/5bd11abeb95ca734c767cfc992ea9be862700614/internal/postgres/manager.go#L64-L97
			if signal := spawnedCompletionSignal(spawn, function, closure, instruction); signal != nil && !ssaflow.DefinitelyNil(signal) {
				signals = append(signals, signal)
			}
		}
	}
	groups, unsettledDone = waitGroupCompletionValues(spawn, function, closure)
	groups = append(groups, deferredCompletionGroups(spawn, function, closure)...)
	return signals, groups, unsettledDone
}

// A deferred helper can finish a group after releasing other worker resources.
// Keep that group as an alternative completion handle, so returning its owner
// is visible even when the worker also sends on an unrelated output channel.
// https://github.com/vbauerster/mpb/blob/ddeb4bb7bcb86e114648760018b10700841a081a/heap_manager.go#L46-L61
func deferredCompletionGroups(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure) []ssa.Value {
	var groups []ssa.Value
	for _, pair := range ssaflow.CallBindings(spawn.Common(), function, closure) {
		group := ssaflow.CapturedBindingValue(pair.Supplied)
		if group == nil || !syntax.NamedType(group.Type(), "sync", "WaitGroup") {
			continue
		}
		// A conditional registration promises completion only on that branch.
		// It cannot create an unconditional obligation for the parent, which may
		// use the same flag to decide whether to wait. Require return coverage,
		// not merely one deferred helper that would call Done if registered.
		// https://github.com/hashicorp/vault-secrets-operator/blob/451a61fc0eda5b26e65dedb03e76fa8ec02b2984/vault/client_factory.go#L890-L909
		unsettled := ssaflow.UnownedReturnFromEntryAssumingNonNil(function, pair.Local, func(instruction ssa.Instruction) bool {
			deferred, ok := instruction.(*ssa.Defer)
			if !ok {
				return false
			}
			if ssaflow.CallMatchesSymbol(deferred.Common(), waitGroupDone) &&
				ssaflow.DefinitelySameValue(ssaflow.CallReceiver(deferred.Common()), pair.Local) {
				return true
			}
			proof := ssaflow.ProveCompletion(ssaflow.CompletionRequest{
				Instruction: deferred, Target: pair.Local, Methods: []string{"Done"},
				Budget: ssaflow.NewSearchBudget(1000),
			})
			return proof.Proven()
		})
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
) ssa.Value { //nolint:ireturn // Completion signals retain their concrete SSA value types.
	if send, ok := instruction.(*ssa.Send); ok {
		// A send followed by further work can announce readiness or progress,
		// not completion. Blocking-producer checks remain independent of this
		// narrower join obligation, and use the full send lifecycle themselves.
		// https://github.com/kubernetes-sigs/cluster-proportional-autoscaler/blob/39dd2288da294e98d683c5619fc3556016df1e76/pkg/autoscaler/autoscaler_server.go#L109-L124
		if !terminalCompletion(send) {
			return nil
		}
		return signalSuppliedAtCall(spawn, function, closure, send.Chan)
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return nil
	}
	if _, launched := instruction.(*ssa.Go); !launched {
		if nested, ok := common.Value.(*ssa.MakeClosure); ok {
			if signal := nestedClosureSignal(nested); signal != nil {
				return signalSuppliedAtCall(spawn, function, closure, signal)
			}
		}
	}
	if ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1 {
		return signalSuppliedAtCall(spawn, function, closure, common.Args[0])
	}
	return nil
}

// signalSuppliedAtCall maps a worker-side channel back to the parent's value.
// A channel selected from a captured aggregate, such as chans[index] in a
// per-shard snapshot, resolves to the aggregate itself: the parent then joins
// by receiving from any part of it and transfers it by handing the aggregate
// on. Matching any element over-approximates joins, which only widens what the
// analyzer accepts.
// https://github.com/nacos-group/nacos-sdk-go/blob/002486583df5ad370ab809cd19dfd97e71b2ef6d/clients/cache/concurrent_map.go#L199-L219
func signalSuppliedAtCall(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	channel ssa.Value,
) ssa.Value { //nolint:ireturn // Completion signals retain their concrete SSA value types.
	if supplied := ssaflow.SpawnedValueAtCall(spawn, function, closure, channel); supplied != nil {
		return supplied
	}
	root := aggregateRoot(channel)
	if root == channel {
		return nil
	}
	for _, pair := range ssaflow.CallBindings(spawn.Common(), function, closure) {
		if ssaflow.ValueAliases(root, pair.Local, map[ssa.Value]bool{}) {
			return ssaflow.CapturedBindingValue(pair.Supplied)
		}
	}
	return nil
}

// aggregateRoot strips element, field, and map selections and the loads
// between them, returning the aggregate a projected value was read from. The
// index operands are deliberately not followed: a loop counter used to select
// an element is not the aggregate that owns it.
func aggregateRoot(value ssa.Value) ssa.Value { //nolint:ireturn // Roots retain their concrete SSA forms.
	for {
		if inner, ok := ssaflow.UnwrapTransparentValue(
			value,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		); ok {
			value = inner
			continue
		}
		switch typed := value.(type) {
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return value
			}
			value = typed.X
		case *ssa.IndexAddr:
			value = typed.X
		case *ssa.FieldAddr:
			value = typed.X
		case *ssa.Index:
			value = typed.X
		case *ssa.Field:
			value = typed.X
		case *ssa.Lookup:
			value = typed.X
		default:
			return value
		}
	}
}

// nestedClosureSignal returns the worker-level value that a synchronously
// invoked inner closure sends on or closes. A deferred inner closure is the
// common shape: `defer func() { done <- recover() }()`.
func nestedClosureSignal(nested *ssa.MakeClosure) ssa.Value { //nolint:ireturn // Join handles retain their concrete SSA value types.
	function, _ := nested.Fn.(*ssa.Function)
	if function == nil {
		return nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			var channel ssa.Value
			if send, ok := instruction.(*ssa.Send); ok {
				channel = send.Chan
			} else if common := ssaflow.InstructionCall(instruction); common != nil &&
				ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1 {
				channel = common.Args[0]
			}
			if channel == nil {
				continue
			}
			for index, free := range function.FreeVars {
				if index < len(nested.Bindings) && ssaflow.ValueAliases(channel, free, map[ssa.Value]bool{}) {
					return ssaflow.CapturedBindingValue(nested.Bindings[index])
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
) (groups []ssa.Value, unsettled ssa.Instruction) {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
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
			group := ssaflow.SpawnedValueAtCall(spawn, function, closure, receiver)
			if group == nil || ssaflow.SameAsAny(group, groups) {
				continue
			}
			if !waitGroupSettlesFunction(function, receiver) {
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

func waitGroupSettlesFunction(function *ssa.Function, receiver ssa.Value) bool {
	hasReturn := false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
		}
	}
	if !hasReturn {
		return false
	}
	// Paths reachable only when the group is nil carry no obligation: without a
	// WaitGroup nothing was added and nothing waits, so a Done guarded by a nil
	// check on the group itself still settles every path that has a group.
	// Vitess passes a group only on the shutdown path that waits for it:
	// https://github.com/vitessio/vitess/blob/44321d8ca0e2b2689e869bc680b6ce6402bba977/go/vt/vttablet/tabletserver/state_manager.go#L605-L631
	return !ssaflow.UnownedReturnFromEntryAssumingNonNil(function, receiver, func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		if common == nil || !ssaflow.CallMatchesSymbol(common, waitGroupDone) ||
			!ssaflow.ValueAliases(ssaflow.CallReceiver(common), receiver, map[ssa.Value]bool{}) {
			return false
		}
		if _, deferred := instruction.(*ssa.Defer); deferred {
			return true
		}
		return terminalCompletion(instruction)
	})
}

// terminalCompletion reports whether only returns can follow a completion
// operation. Later work cannot be joined by observing an earlier signal.
func terminalCompletion(done ssa.Instruction) bool {
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

func bufferedLocalChannel(function *ssa.Function, signal ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			created, ok := instruction.(*ssa.MakeChan)
			if !ok || !carries(ssaflow.NewReachingWalk(carryForms), signal, created) {
				continue
			}
			size, constantSize := created.Size.(*ssa.Const)
			return !constantSize || size.Value == nil || constant.Sign(size.Value) > 0
		}
	}
	return false
}
