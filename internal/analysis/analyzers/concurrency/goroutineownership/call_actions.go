package goroutineownership

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/syntax"
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
	if ssacall.HasLibraryContract(common, ssacall.ContractTestingCleanup) {
		proof := analysis.testingCleanupAction(common)
		return proof.action, proof.reason
	}
	if ssacall.HasLibraryContract(common, ssacall.ContractGoMockReturn) && analysis.anyArgumentConsumes(common) {
		// gomock.Return publishes its configured results, but broad argument
		// containment does not prove the exact stream is among those results.
		// https://github.com/uber-go/mock/blob/539d81c0f42174d17e8f91abcb869bed37605a15/gomock/call.go#L185-L205
		return actionUnknown, reasonLabelGoMockReturn
	}
	callee, closure := ssacall.DirectCallee(common)
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
	if !ssacall.CallMatchesSymbol(common, waitGroupWait) {
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
	return ssacall.CallMatchesSymbol(common, waitGroupAdd) || ssacall.CallMatchesSymbol(common, waitGroupDone) ||
		ssacall.CallMatchesSymbol(common, waitGroupGo) || ssacall.CallMatchesSymbol(common, waitGroupWait)
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

// Helper-call evidence maps caller handles into a source-visible helper and
// distinguishes exact completion from possible ownership. Binding preparation,
// recursive effects and exact caller identity share one candidate allowance;
// incomplete selection is opaque rather than a completed absence of handoff.

type helperCallProof struct {
	action ownershipAction
	reason goroutineOwnershipReason
}

func (analysis *spawnAnalysis) helperCallResult(action ownershipAction, budget *proofs.SearchBudget) helperCallProof {
	if budget.Exhausted() {
		budget.Observe(proofs.EvidenceBudgetExhausted, analysis.spawn.Pos(), func() map[string]string {
			return map[string]string{"phase": "helper-call"}
		})
		return helperCallProof{action: actionUnknown, reason: reasonHelperCallBudgetExhausted}
	}
	return helperCallProof{action: action, reason: reasonLabelHelper}
}

// helperAction follows every tracked value that the call site supplies to a
// source-visible callee, whether as an argument or a captured variable.
func (analysis *spawnAnalysis) helperAction(
	common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure, values []trackedValue,
) helperCallProof {
	budget := analysis.budget()
	result := actionNone
	var search *helperSearch
	for pair := range ssacall.CallBindingsWithin(common, callee, closure, budget) {
		for _, tracked := range values {
			if !budget.Spend() {
				return analysis.helperCallResult(actionUnknown, budget)
			}
			carried := bindingCarries(pair.Supplied, tracked.value)
			projected := ssaflow.ValueIsAccessPathFrom(tracked.value, pair.Supplied)
			if !carried && !projected {
				continue
			}
			// The memo key retains the formal/capture and tracked kind. Caller
			// identity is checked separately for each supplied binding.
			if search == nil {
				search = newHelperSearchWithin(budget)
				search.concurrency, _ = summaryKnowledge.Provider(analysis.pass).Concurrency()
			}
			action := boundHelperAction(pair.Supplied, tracked, search.use(callee, pair.Local, tracked.kind), budget)
			if budget.Exhausted() {
				return analysis.helperCallResult(actionUnknown, budget)
			}
			result = strongerAction(result, action)
			// One exact completion alternative covers the worker. Later opaque
			// handles cannot weaken it; stop before preparing unrelated bindings.
			if result == actionJoin {
				return analysis.helperCallResult(result, budget)
			}
		}
	}
	return analysis.helperCallResult(result, budget)
}

// Owner coverage establishes a lifecycle call, not observation of worker
// completion. A completion handle also needs exact binding: containment includes
// old stores and aggregate projections, which supply only possible shutdown.
// https://github.com/jech/galene/blob/6d9338e909fdecdd906150e4dda34e10d9869654/rtpconn/webclient.go#L878-L894
func boundHelperAction(supplied ssa.Value, tracked trackedValue, effect ownershipAction, budget *proofs.SearchBudget) ownershipAction {
	if effect != actionJoin {
		return effect
	}
	if tracked.kind == trackedOwner {
		return actionUnknown
	}
	if !heapmodel.NewStorage(budget).Same(supplied, tracked.value).Proven() {
		return actionUnknown
	}
	return actionJoin
}
