package ssainfer

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Returned cleanup records an exact relation, not historical containment:
// invoking one callback result completes a particular parameter or sibling
// result. Every return must establish it. Mutable captures, alternate no-op
// callbacks, different factory calls, and truncated searches prove nothing.

// ReturnedCleanupRelation ties a callback result to a parameter or result of
// the same factory invocation. TargetIsResult selects the target's namespace.
type ReturnedCleanupRelation struct {
	CallbackResult int
	Target         int
	TargetIsResult bool
}

// ReturnedCleanupLookup supplies positive factory relations for one exact
// method or callback-invocation contract. Missing relations mean unknown.
type ReturnedCleanupLookup func(*ssa.Function, string, bool) []ReturnedCleanupRelation

type returnedCleanupKey struct {
	function *ssa.Function
	relation ReturnedCleanupRelation
	invoke   bool
}

// ProveReturnedCleanup proves a factory relation using the completion request's
// methods or InvokeTarget mode, budget, and imported-summary policies. Target
// and Instruction are unused: relation identifies values inside the factory.
func ProveReturnedCleanup(function *ssa.Function, relation ReturnedCleanupRelation, request CompletionRequest) ssaflow.CompletionProof {
	methods := request.Methods
	if request.InvokeTarget {
		if len(methods) != 0 {
			return ssaflow.CompletionProof{}
		}
		methods = []string{""}
	}
	for _, method := range methods {
		search := newCompletionSearch(method, CoverageEveryReturn, request.Budget)
		search.exactTarget, search.exactInvocation, search.invokeTarget = true, request.InvokeTarget, request.InvokeTarget
		search.summarized, search.returnedSummaries = request.Summarized, request.ReturnedSummaries
		search.callContract = request.CallContract
		if search.returnedRelation(function, relation) {
			return ssaflow.CompletionProof{Proof: ssaflow.Proof{
				State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceReturnedDeferredCleanup, Method: method, Provenance: ssaflow.EvidenceFromLocalSSA,
			}}
		}
	}
	return ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
}

func (search *completionSearch) returnedCallCompletes(instruction ssa.Instruction, target ssa.Value) (launchKind, bool) {
	var kind launchKind
	switch instruction.(type) {
	case *ssa.Call:
		kind = launchCalled
	case *ssa.Defer:
		kind = launchDeferred
	default:
		return launchNone, false
	}
	common := ssaflow.InstructionCall(instruction)
	return kind, common != nil && search.returnedValueCompletes(common.Value, target)
}

func (search *completionSearch) returnedValueCompletes(callback, target ssa.Value) bool {
	factory, callbackIndex, ok := ssaflow.CallResultSource(callback)
	if !ok || !search.budget.Spend() {
		return false
	}
	function := ssaflow.ResolvedCallee(factory.Common())
	if function == nil {
		return false
	}
	storage := NewStorage(search.budget)
	for index, argument := range factory.Common().Args {
		relation := ReturnedCleanupRelation{CallbackResult: callbackIndex, Target: index}
		if storage.Same(argument, target).Proven() && search.returnedRelation(function, relation) {
			return true
		}
		// A factory may return a callback that invokes a supplied callback.
		// If that supplied callback is bound to the exact cleanup method on
		// target, invocation of the returned callback settles target as well.
		// https://github.com/chengshiwen/influxdb-cluster/blob/194fcaae32ff8e5d7486d60590af55f278e3cee7/pkg/errors/error_capture.go#L14-L20
		if search.method != "" && exactCallbackCallsMethod(argument, search.method, target) {
			invoke := *search
			invoke.method, invoke.invokeTarget, invoke.exactInvocation = "", true, true
			if invoke.returnedRelation(function, relation) {
				return true
			}
		}
	}
	for index := range function.Signature.Results().Len() {
		if !search.budget.Spend() {
			return false
		}
		result := ssaflow.CallResult(factory, index)
		if result != nil && storage.Same(result, target).Proven() && search.returnedRelation(function, ReturnedCleanupRelation{
			CallbackResult: callbackIndex, Target: index, TargetIsResult: true,
		}) {
			return true
		}
	}
	return false
}

// An opaque wrapper may discard a callback even if its result depends on the
// callback argument. This bridge accepts only an exact closure, through
// transparent conversions and every phi alternative; it never treats an
// arbitrary call result as preserving the bound cleanup.
func exactCallbackCallsMethod(value ssa.Value, method string, target ssa.Value) bool {
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType |
		ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	return ssaflow.NewReachingWalk(forms).Every(value, func(_ ssaflow.ReachingWalk, leaf ssa.Value) bool {
		_, closure := leaf.(*ssa.MakeClosure)
		return closure && ValueCallsMethod(leaf, method, target)
	})
}

func (search *completionSearch) returnedRelation(function *ssa.Function, relation ReturnedCleanupRelation) bool {
	if function == nil || relation.CallbackResult < 0 || relation.CallbackResult >= function.Signature.Results().Len() || relation.Target < 0 {
		return false
	}
	if len(function.Blocks) == 0 && search.returnedSummaries != nil {
		return slices.Contains(search.returnedSummaries(function, search.method, search.invokeTarget), relation)
	}
	key := returnedCleanupKey{function: function, relation: relation, invoke: search.invokeTarget}
	return search.returnedMemo.Summarize(key, function, search.budget, func() bool {
		return search.returnedRelationBody(function, relation)
	}, func(ssaflow.SummaryUnavailable, bool) bool { return false })
}

func (search *completionSearch) returnedRelationBody(function *ssa.Function, relation ReturnedCleanupRelation) bool {
	witness := false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !search.budget.Spend() {
				return false
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			var target ssa.Value
			if relation.TargetIsResult {
				target = returnedCleanupValue(returned, relation.Target, search.budget)
			} else if relation.Target < len(function.Params) {
				target = function.Params[relation.Target]
			}
			callback := returnedCleanupValue(returned, relation.CallbackResult, search.budget)
			if target == nil || callback == nil || !search.returnedCallbackCompletes(callback, target, returned) {
				return false
			}
			witness = true
		}
	}
	return witness
}

func returnedCleanupValue(returned *ssa.Return, index int, budget *ssaflow.SearchBudget) ssa.Value { //nolint:ireturn // Preserve exact SSA values.
	if index < 0 || index >= len(returned.Results) {
		return nil
	}
	value := returned.Results[index]
	if resolved := NewStorage(budget).Resolve(value); resolved.Proven() {
		return resolved.Value
	}
	return value
}

func (search *completionSearch) returnedCallbackCompletes(callback, target ssa.Value, returned *ssa.Return) bool {
	if search.invokeTarget && DefinitelySameValue(callback, target) {
		return true
	}
	if closure, ok := callback.(*ssa.MakeClosure); ok {
		function, ok := closure.Fn.(*ssa.Function)
		if !ok {
			return false
		}
		strict := *search
		strict.exactTarget = true
		strict.condition = completionCondition{}
		return strict.calleeCompletes(completionCallee{
			function: function, closure: closure, launch: launchCallback, invocation: returned,
		}, target, returned).proven
	}
	return search.returnedValueCompletes(callback, target)
}
