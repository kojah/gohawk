package goroutineownership

import (
	"go/constant"
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Obligation evidence identifies what a launched function promises its
// parent: a channel it sends on or closes, a WaitGroup it settles, or, for the
// detached audit only, a lifecycle owner it runs on. Values are resolved back
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
	// trackedOwner is a captured value with a lifecycle method. It supports only
	// the opt-in detached audit and is recognized by method name, which is why
	// it never contributes to the default check.
	trackedOwner
)

type trackedValue struct {
	value ssa.Value
	kind  trackedKind
}

type spawnAnalysis struct {
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
}

func newSpawnAnalysis(function *ssa.Function, spawn *ssa.Go, config goroutineOwnershipConfig) *spawnAnalysis {
	analysis := &spawnAnalysis{
		function: function,
		spawn:    spawn,
		config:   config,
		actions:  make(map[ssa.Instruction]ownershipAction),
	}
	analysis.signals, analysis.groups, analysis.unsettledDone = spawnedCompletionValues(spawn)
	if config.mode != goroutineModeJoin {
		analysis.owners = spawnedLifecycleOwners(spawn)
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
	if config.mode != goroutineModeJoin && len(analysis.signals) == 0 && len(analysis.groups) == 0 && analysis.unsettledDone == nil {
		analysis.checkID = check.GoroutineDetached
	}
	return analysis
}

func spawnedFunction(spawn *ssa.Go) (*ssa.Function, *ssa.MakeClosure) {
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	// Generic calls may point at an instantiated wrapper whose body does not
	// expose the receive. The origin has the same parameter positions and the
	// source body needed to prove that the helper joins the signal.
	if function != nil && function.Origin() != nil {
		function = function.Origin()
	}
	return function, closure
}

func spawnedCompletionValues(spawn *ssa.Go) (signals, groups []ssa.Value, unsettledDone ssa.Instruction) {
	function, closure := spawnedFunction(spawn)
	if function == nil {
		return nil, nil, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if signal := spawnedCompletionSignal(spawn, function, closure, instruction); signal != nil {
				signals = append(signals, signal)
			}
		}
	}
	groups, unsettledDone = waitGroupCompletionValues(spawn, function, closure)
	return signals, groups, unsettledDone
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
	for _, pair := range suppliedValues(spawn.Common(), function, closure) {
		if ssaflow.ValueAliases(root, pair.local, map[ssa.Value]bool{}) {
			return ssaflow.CapturedBindingValue(pair.supplied)
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
	return !ssaflow.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		if common == nil || !ssaflow.CallMatchesSymbol(common, waitGroupDone) ||
			!ssaflow.ValueAliases(ssaflow.CallReceiver(common), receiver, map[ssa.Value]bool{}) {
			return false
		}
		if _, deferred := instruction.(*ssa.Defer); deferred {
			return true
		}
		return waitGroupDoneIsTerminal(instruction)
	})
}

// waitGroupDoneIsTerminal reports whether only returns can follow done.
func waitGroupDoneIsTerminal(done ssa.Instruction) bool {
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
			if _, returned := current.block.Instrs[current.index].(*ssa.Return); !returned {
				return false
			}
			continue
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
			if !ok || !carries(signal, created, map[ssa.Value]bool{}) {
				continue
			}
			size, constantSize := created.Size.(*ssa.Const)
			return !constantSize || size.Value == nil || constant.Sign(size.Value) > 0
		}
	}
	return false
}
