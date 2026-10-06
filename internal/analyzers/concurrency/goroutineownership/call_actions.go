package goroutineownership

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Call classification distinguishes synchronous joins from launched or opaque
// consumption. Exact group/library contracts stay separate from broad receiver
// or capture identity, which can explain uncertainty but cannot prove a join.

func (analysis *spawnAnalysis) callAction(instruction ssa.Instruction, common *ssa.CallCommon) (ownershipAction, goroutineOwnershipReason) {
	if common == nil {
		return actionNone, reasonNone
	}
	// A launched observer runs independently of the caller. Handle it before
	// receiver, summary and library contracts that describe synchronous calls.
	if _, launched := instruction.(*ssa.Go); launched {
		return analysis.opaqueCallAction(common, reasonLabelLaunchedHelper)
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		// append retains its arguments in a slice the caller keeps; the other
		// builtins only observe a channel or its capacity.
		if builtin.Name() == "append" && analysis.anyArgumentConsumes(common) {
			return actionUnknown, reasonLabelAppended
		}
		return actionNone, reasonNone
	}
	if action, reason := analysis.directJoinAction(common); action != actionNone {
		return action, reason
	}
	owner := analysis.closesRetainedWorkerOwner(instruction, common)
	if owner.Reason == proofs.EvidenceBudgetExhausted {
		return actionUnknown, reasonRetainedOwnerBudgetExhausted
	}
	if owner.Proven() {
		return actionUnknown, reasonLabelClosesRetainedOwner
	}
	if action := analysis.pipePeerAction(instruction, common); action != actionNone {
		return action, reasonLabelPipePeer
	}
	if proof := analysis.summarizedJoin(instruction); proof.action != actionNone {
		return proof.action, proof.reason
	}
	if analysis.waitGroupBookkeeping(common) {
		return actionNone, reasonNone
	}
	if ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
		proof := analysis.testingCleanupAction(common)
		return proof.action, proof.reason
	}
	if ssaflow.HasLibraryContract(common, ssaflow.ContractGoMockReturn) && analysis.anyArgumentConsumes(common) {
		// gomock.Return publishes its configured results, but broad argument
		// containment does not prove the exact stream is among those results.
		// https://github.com/uber-go/mock/blob/539d81c0f42174d17e8f91abcb869bed37605a15/gomock/call.go#L185-L205
		return actionUnknown, reasonLabelGoMockReturn
	}
	callee, closure := ssaflow.DirectCallee(common)
	if callee == nil || len(callee.Blocks) == 0 {
		// An opaque callee may retain the value.
		if callee == nil {
			return analysis.opaqueCallAction(common, reasonLabelDynamicCallee)
		}
		return analysis.opaqueCallAction(common, reasonLabelCalleeWithoutBody)
	}
	proof := analysis.helperAction(common, callee, closure, analysis.tracked)
	return proof.action, proof.reason
}

// Opaque and launched calls share the same handoff boundary. Positive argument
// or capture consumption, or an interrupted capture search, leaves ownership
// uncertain without establishing a join.
func (analysis *spawnAnalysis) opaqueCallAction(
	common *ssa.CallCommon, reason goroutineOwnershipReason,
) (ownershipAction, goroutineOwnershipReason) {
	if analysis.anyArgumentConsumes(common) || analysis.closureConsumes(common.Value) {
		return actionUnknown, reason
	}
	return actionNone, reasonNone
}

// Wait observes completion only on the exact settling group. A lifecycle
// method on a captured owner supplies possible shutdown participation; neither
// its name nor receiver identity proves observation of this worker's completion.
func (analysis *spawnAnalysis) directJoinAction(common *ssa.CallCommon) (ownershipAction, goroutineOwnershipReason) {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil {
		return actionNone, reasonNone
	}
	if !ssaflow.CallMatchesSymbol(common, waitGroupWait) {
		if lifecycleMethod(ssaflow.CallName(common)) && ownerReceiver(receiver, analysis.owners) {
			return actionUnknown, reasonLabelOwnerLifecycle
		}
		return actionNone, reasonNone
	}
	storage := heapmodel.NewStorage(analysis.budget())
	for _, target := range analysis.groups {
		if storage.Same(receiver, target).Proven() {
			return actionJoin, reasonLabelDirectJoin
		}
	}
	if heapmodel.MayAliasAny(receiver, analysis.groups) || storage.Budget().Exhausted() {
		return actionUnknown, reasonLabelPossibleJoin
	}
	return actionNone, reasonNone
}

var waitGroupAdd = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Add"})

var waitGroupGo = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Go"})

// waitGroupBookkeeping recognizes the documented sync.WaitGroup counter
// methods on a tracked group. They neither observe completion nor let the
// group escape, so they must not make the group opaque.
func (analysis *spawnAnalysis) waitGroupBookkeeping(common *ssa.CallCommon) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || !heapmodel.MayAliasAny(receiver, analysis.groups) && !analysis.unsettledGroup(receiver) {
		return false
	}
	return waitGroupMethod(common)
}

func waitGroupMethod(common *ssa.CallCommon) bool {
	return ssaflow.CallMatchesSymbol(common, waitGroupAdd) || ssaflow.CallMatchesSymbol(common, waitGroupDone) ||
		ssaflow.CallMatchesSymbol(common, waitGroupGo) || ssaflow.CallMatchesSymbol(common, waitGroupWait)
}

// unsettledGroup keeps an early-Done WaitGroup out of the opaque set so the
// readiness bookkeeping cannot hide an independent completion obligation.
func (analysis *spawnAnalysis) unsettledGroup(receiver ssa.Value) bool {
	if analysis.unsettledDone == nil {
		return false
	}
	pointer, ok := receiver.Type().Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "sync", "WaitGroup")
}

// testingCleanupAction treats a testing Cleanup callback like a deferred
// helper: testing guarantees that it runs after the test completes, so a join
// on every path through the callback settles the worker.
// https://github.com/charmbracelet/crush/blob/6fa9e6905041c32ffceb1c9b1a3189b3db1eec07/internal/server/socket_test.go#L162-L177
func (analysis *spawnAnalysis) testingCleanupAction(common *ssa.CallCommon) helperCallProof {
	result := actionNone
	for _, argument := range common.Args {
		closure, ok := argument.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		if callee, _ := closure.Fn.(*ssa.Function); callee != nil {
			proof := analysis.helperAction(nil, callee, closure, analysis.tracked)
			if proof.reason == reasonHelperCallBudgetExhausted {
				return proof
			}
			result = strongerAction(result, proof.action)
		}
	}
	return helperCallProof{action: result, reason: reasonLabelTestingCleanup}
}

func (analysis *spawnAnalysis) anyArgumentConsumes(common *ssa.CallCommon) bool {
	return slices.ContainsFunc(common.Args, analysis.consumes)
}

// Closure choices establish possible consumption, never a unique callee or a
// join. Only phi alternatives are transparent here; converted or loaded function
// values retain their existing opaque boundary. Interrupted capture discovery
// must also leave ownership unknown, rather than establish absent consumption.
// https://github.com/Debian/dcs/blob/567a9be49163cbf731f25bf79890f692e04d22d9/internal/sourcebackend/sourcebackend.go#L433-L644
func (analysis *spawnAnalysis) closureConsumes(value ssa.Value) bool {
	budget := analysis.budget()
	consumes := ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(budget).Any(value, func(_ ssaflow.ReachingWalk, current ssa.Value) bool {
		closure, ok := current.(*ssa.MakeClosure)
		if !ok {
			return false
		}
		for _, binding := range closure.Bindings {
			for _, tracked := range analysis.tracked {
				if !budget.Spend() {
					return false
				}
				if bindingCarries(binding, tracked.value) {
					return true
				}
			}
		}
		return false
	})
	return consumes || budget.Exhausted()
}
