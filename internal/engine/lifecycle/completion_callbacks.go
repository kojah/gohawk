package lifecycle

import (
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Callback capabilities describe values that may carry cleanup, not a promise
// that their owners invoke it. Origin and stored-value traversal use the same
// completion engine for callback bodies; request cutoff supplies no capability.

// ValueCallsMethod reports whether value is, or carries, a callback that
// calls method on target when invoked: a function literal whose body
// completes the target, a bound method value, or such a callback held in a
// local, passed through a call result, or merged by a phi.
func ValueCallsMethod(value ssa.Value, method string, target ssa.Value) bool {
	return ProveValueCallsMethodWithin(value, method, target, nil).Proven()
}

// ProveValueCallsMethodWithin asks whether value may carry a callback whose
// body completes method on target. It retains ValueCallsMethod's any-origin
// and callback-preserving wrapper policies; it proves neither invocation nor
// that every possible callback completes. Value, referrer and callee queries
// share budget. A cutoff is unknown; nil retains the default unbounded search.
func ProveValueCallsMethodWithin(value ssa.Value, method string, target ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	matched := newCompletionSearch(method, CoverageEveryReturn, budget).valueCallsMethod(value, target)
	if budget.Exhausted() {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	if matched {
		return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceCallbackCompletion, Method: method}
	}
	return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}
}

func (search *completionSearch) valueCallsMethod(value, target ssa.Value) bool {
	return search.callbackCapability(search.callbackValues, value, target)
}

// Any keeps the historical may-carry policy across phi alternatives. Its fold
// owns wrapper/origin recursion; callee recursion stays in the completion memo.
// A revisited origin supplies no capability and never proves invocation.
func (search *completionSearch) callbackCapability(walk ssaflow.ReachingWalk, value, target ssa.Value) bool {
	return walk.Any(value, func(next ssaflow.ReachingWalk, leaf ssa.Value) bool {
		switch typed := leaf.(type) {
		case *ssa.MakeClosure:
			callees, ok := closureCallees(typed, launchCallback, search.budget)
			return ok && search.calleeCompletes(callees[0], target, nil).proven
		case *ssa.Alloc:
			return search.storedValueCallsMethod(next, typed, target)
		case *ssa.UnOp:
			return search.storedValueCallsMethod(next, typed.X, target)
		case *ssa.Call:
			// This is a possible retained capability, not an exact factory relation:
			// a wrapper receiving a callback may carry it in its returned value.
			return slices.ContainsFunc(typed.Common().Args, func(argument ssa.Value) bool {
				return search.callbackCapability(next, argument, target)
			})
		}
		return false
	})
}

func (search *completionSearch) storedValueCallsMethod(walk ssaflow.ReachingWalk, address, target ssa.Value) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		if !search.budget.Spend() {
			return false
		}
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address && search.callbackCapability(walk, store.Val, target) {
			return true
		}
	}
	return false
}

// Callback invocation uses the ordinary completion engine's exact identity,
// binding and coverage rules. Observing a spawned wrapper's body says what
// that wrapper promises; it never credits its launch as caller completion.

// CallInvokesArgumentOnEveryReturn reports whether a synchronous or deferred
// callee invokes the exact target before every normal return.
func CallInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	return ProveCompletion(CompletionRequest{
		Instruction: instruction, Target: target, InvokeTarget: true,
		Budget: proofs.NewSearchBudget(proofs.QueryBudget),
	}).Proven()
}

// ProveSpawnedInvocation observes the launched wrapper's body and requires
// synchronous invocation of target before every normal return. The caller's
// budget bounds this query; a cutoff stays unknown and proves no invocation.
func ProveSpawnedInvocation(spawn *ssa.Go, target ssa.Value, budget *proofs.SearchBudget) proofs.CompletionProof {
	if spawn == nil || target == nil {
		return proofs.CompletionProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	request := CompletionRequest{Instruction: spawn, Target: target, InvokeTarget: true, Budget: budget}
	function, closure := ssacall.DirectCallee(spawn.Common())
	if function == nil || len(function.Blocks) == 0 {
		return request.unprovenCompletion(false, false, false)
	}
	search := newCompletionSearch("", CoverageEveryReturn, budget)
	search.exactTarget, search.exactInvocation, search.invokeTarget = true, true, true
	// Examine the body independently of its launch, as for an uninvoked
	// callback. Nested launches still pass through ordinary completion and
	// cannot establish synchronous invocation before the wrapper returns.
	answer := search.calleeCompletes(completionCallee{
		launch: launchCallback, common: spawn.Common(), closure: closure,
		function: function, invocation: spawn,
	}, target, spawn)
	if answer.proven {
		return answer.proof("")
	}
	return request.unprovenCompletion(answer.available, *search.incomplete, *search.inCycle)
}

// Closure evidence follows captured values through bindings, stored callbacks,
// and helper calls. These helpers accept only concrete SSA relationships and
// stop at cycles or unmodeled indirection so callers can treat a match as proof.

// DeferredClosureCallsValue reports whether a deferred closure calls target.
func DeferredClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsValue(instruction, target)
}

// DeferredClosureInvokesArgumentOnEveryReturn reports whether a deferred
// closure delegates target to a helper that invokes it on every normal path.
func DeferredClosureInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	function, closure := ssacall.DirectCallee(common)
	if function == nil {
		return false
	}
	bindings := ssacall.CallBindings(common, function, closure)
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			for _, binding := range bindings {
				// Captures name cells read by the deferred body; arguments
				// are values evaluated when the defer is registered. Keep
				// their matching policies distinct after pairing them once.
				matches := heapmodel.MayAlias(binding.Supplied, target)
				if binding.Captured {
					matches = heapmodel.CapturedBindingMatches(binding.Supplied, target)
				}
				if matches && CallInvokesArgumentOnEveryReturn(candidate, binding.Local) {
					return true
				}
			}
		}
	}
	return false
}

// ClosureCallsValue reports whether a call-like closure or created callback calls target.
func ClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	var closure *ssa.MakeClosure
	if created, ok := instruction.(*ssa.MakeClosure); ok {
		if created.Referrers() == nil || len(*created.Referrers()) == 0 {
			return false
		}
		closure = created
	} else if common := ssaflow.InstructionCall(instruction); common != nil {
		closure, _ = common.Value.(*ssa.MakeClosure)
	}
	if closure == nil {
		return false
	}
	return closureCallsValue(closure, target)
}
