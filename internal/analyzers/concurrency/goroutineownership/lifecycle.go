package goroutineownership

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Lifecycle evidence bounds a worker through exact caller-owned signals,
// cleanup, or one-hop WaitGroup dependencies. Uncertain participation is not a
// join, and absent ownership never establishes an obligation on its own.

// A straight-line Wait followed only by completion closes relays one exact
// group. Waiting independently on that group is an alternative completion
// handle; extra calls, sends, defers, or control flow make that inference opaque.
// https://github.com/ConduitIO/conduit/blob/9946a19b9fff997675f78bbc5ff437e760d39f4f/pkg/lifecycle/stream/parallel.go#L94-L103
func (analysis *spawnAnalysis) relayCompletionGroup(budget *proofs.SearchBudget) ssa.Value { //nolint:ireturn // Retains the caller's exact group identity.
	function, closure := resolveSpawnedFunction(analysis.pass, analysis.spawn, budget)
	if len(analysis.signals) == 0 || function == nil || len(function.Blocks) != 1 || len(function.Blocks[0].Instrs) > 64 {
		return nil
	}
	var group ssa.Value
	closed := false
	for _, instruction := range function.Blocks[0].Instrs {
		if !budget.Spend() {
			return nil
		}
		switch typed := instruction.(type) {
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return nil
			}
		case *ssa.Call:
			common := typed.Common()
			switch {
			case ssaflow.CallMatchesSymbol(common, waitGroupWait) && group == nil:
				group = completionValueAtCall(analysis.spawn, function, closure, ssaflow.CallReceiver(common), budget)
			case group != nil && ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1:
				signal := completionValueAtCall(analysis.spawn, function, closure, common.Args[0], budget)
				if !analysis.isSignalWithin(signal, budget) {
					return nil
				}
				closed = true
			default:
				return nil
			}
		case *ssa.Return, *ssa.DebugRef:
		default:
			return nil
		}
	}
	if !closed {
		return nil
	}
	return group
}

// A relay can depend on an existing external queue participant or on a worker
// already covered by the local cancellation proof. This is one-hop uncertainty,
// never group-count arithmetic or proof that every participant completes.
// https://github.com/buchgr/bazel-remote/blob/a69b6b5ed933234d93b489ffd216bee5bb74aa06/cache/disk/findmissing.go#L122-L143
// https://github.com/HM2899/grokcli-2api/blob/33a106d902d7627d1cdbc3359768a029112ea808/internal/proxy/chat.go#L445-L565
func (analysis *spawnAnalysis) relayDependencyUncertain(budget *proofs.SearchBudget) bool {
	if analysis.relayGroup == nil {
		return false
	}
	for instruction := range ssaflow.InstructionsWithin(analysis.function, budget) {
		send, sends := instruction.(*ssa.Send)
		worker, launches := instruction.(*ssa.Go)
		// Only a send or another worker can supply this dependency witness.
		// Avoid spending ordered reachability on unrelated instructions.
		participant := sends || launches
		if instruction == analysis.spawn || !participant || !cfg.InstructionMayFollowWithin(instruction, analysis.spawn, budget) {
			continue
		}
		if sends && lifecycle.MayContainValue(send.X, analysis.relayGroup) {
			return true
		}
		if !launches {
			continue
		}
		function, closure := resolveSpawnedFunction(analysis.pass, worker, budget)
		if function == nil {
			continue
		}
		groups, _ := waitGroupCompletionValues(worker, function, closure, budget)
		if heapmodel.MayAliasAny(analysis.relayGroup, groups) && goroutineReceivesLocallyCanceledContext(analysis.pass, worker, budget) {
			return true
		}
	}
	return false
}

// synctestOwnsGoroutine recognizes a worker launched from the callback passed
// to synctest.Test, which waits for every goroutine in its bubble:
// https://github.com/golang/go/blob/8af21751f066eced273ca3ce49506b366847c623/src/testing/synctest/synctest.go#L275-L293
func synctestOwnsGoroutine(function *ssa.Function) bool {
	if function == nil || function.Parent() == nil {
		return false
	}
	for _, block := range function.Parent().Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if !ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("testing/synctest", "Test")) {
				continue
			}
			for _, argument := range common.Args {
				if callbackFunction(argument) == function {
					return true
				}
			}
		}
	}
	return false
}

func callbackFunction(value ssa.Value) *ssa.Function {
	function, _ := callbackTarget(value)
	return function
}

// spawnedLifecycleOwners returns the receiver and captured values that expose
// a lifecycle method. This is the only name-based evidence in the analyzer and
// it can only suppress a diagnostic, never establish an obligation. A WaitGroup
// is excluded so its Wait cannot bypass the terminal Done proof.
func spawnedLifecycleOwners(pass *analysis.Pass, spawn *ssa.Go, budget *proofs.SearchBudget) []ssa.Value {
	var owners []ssa.Value
	if receiver := ssaflow.CallReceiver(spawn.Common()); lifecycleOwnerWithin(receiver, budget) {
		owners = append(owners, receiver)
	}
	_, closure := resolveSpawnedFunction(pass, spawn, budget)
	if closure == nil {
		return owners
	}
	for _, binding := range closure.Bindings {
		if !budget.Spend() {
			return owners
		}
		if value := ssaflow.CapturedBindingValueWithin(binding, budget); lifecycleOwnerWithin(value, budget) {
			owners = append(owners, value)
		}
	}
	return owners
}

func lifecycleOwnerWithin(value ssa.Value, budget *proofs.SearchBudget) bool {
	if value == nil || syntax.NamedType(value.Type(), "sync", "WaitGroup") {
		return false
	}
	for method := range types.NewMethodSet(value.Type()).Methods() {
		if !budget.Spend() {
			return false
		}
		if lifecycleMethod(method.Obj().Name()) {
			return true
		}
	}
	return false
}

// ownerReceiver matches a lifecycle call's receiver against the tracked
// owners. A type assertion on the captured interface value denotes the same
// object, so closing the asserted listener stops the worker that captured the
// interface. NATS asserts its listener before deferring the close:
// https://github.com/nats-io/nats.go/blob/850f889cf3d63bfd1a549ab9af59f0145146fb41/nats_test.go#L1288-L1301
func ownerReceiver(receiver ssa.Value, owners []ssa.Value) bool {
	if heapmodel.MayAliasAny(receiver, owners) {
		return true
	}
	if extract, ok := receiver.(*ssa.Extract); ok {
		receiver = extract.Tuple
	}
	asserted, ok := receiver.(*ssa.TypeAssert)
	return ok && heapmodel.MayAliasAny(asserted.X, owners)
}

func lifecycleMethod(name string) bool {
	switch strings.ToLower(name) {
	case "close", "kill", "shutdown", "stop", "wait":
		return true
	default:
		return false
	}
}
