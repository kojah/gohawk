package lifecycle

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Callee resolution supplies a complete, ordered set of visible callback bodies.
// Every origin and its stable-storage lookup share the completion allowance;
// a truncated set cannot establish that all possible callees complete.

// resolveCallees returns every body the instruction may run, or ok=false when
// some path reaches a callee the analysis cannot see. A deferred callback may
// be resolved through the documented sync.OnceFunc contract; a callback
// invoked now may not, because an earlier invocation could already have
// consumed the wrapper.
func resolveCallees(instruction ssa.Instruction, budget *ssaflow.SearchBudget) ([]completionCallee, bool) {
	if !budget.Spend() {
		return nil, false
	}
	switch typed := instruction.(type) {
	case *ssa.Defer:
		return calleesOf(typed.Common(), launchDeferred, instruction, true, budget)
	case *ssa.Go:
		return calleesOf(typed.Common(), launchStarted, instruction, false, budget)
	case *ssa.Call:
		common := typed.Common()
		if ssaflow.CallMatchesSymbol(common, waitGroupGoMethod) && len(common.Args) == 2 {
			return closureCallees(common.Args[1], launchStarted, budget)
		}
		if ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) && len(common.Args) > 0 {
			// testing.TB guarantees that Cleanup callbacks run when the test
			// and its subtests complete, so a registered callback is deferred.
			return closureCallees(common.Args[len(common.Args)-1], launchDeferred, budget)
		}
		return calleesOf(common, launchCalled, instruction, false, budget)
	}
	return nil, false
}

func calleesOf(
	common *ssa.CallCommon, launch launchKind, invocation ssa.Instruction, allowOnceFunc bool, budget *ssaflow.SearchBudget,
) ([]completionCallee, bool) {
	if !budget.Spend() {
		return nil, false
	}
	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		function, _ := closure.Fn.(*ssa.Function)
		return []completionCallee{{launch: launch, common: common, closure: closure, function: function}}, function != nil
	}
	if function := common.StaticCallee(); function != nil {
		return []completionCallee{{launch: launch, common: common, function: function}}, true
	}
	closures, ok := exactCallbacks(common.Value, invocation, allowOnceFunc, budget)
	if !ok {
		return nil, false
	}
	result := make([]completionCallee, 0, len(closures))
	for _, closure := range closures {
		if !budget.Spend() {
			return nil, false
		}
		function, _ := closure.Fn.(*ssa.Function)
		if function == nil {
			return nil, false
		}
		result = append(result, completionCallee{launch: launch, common: common, closure: closure, function: function})
	}
	return result, true
}

func closureCallees(value ssa.Value, launch launchKind, budget *ssaflow.SearchBudget) ([]completionCallee, bool) {
	if !budget.Spend() {
		return nil, false
	}
	closure, ok := value.(*ssa.MakeClosure)
	if !ok {
		return nil, false
	}
	function, _ := closure.Fn.(*ssa.Function)
	return []completionCallee{{launch: launch, closure: closure, function: function}}, function != nil
}

// exactCallbacks resolves a callback value to the function literals it may
// hold. Loads require one dominating store, phi edges must all resolve, and
// other call results are opaque.
func exactCallbacks(value ssa.Value, invocation ssa.Instruction, allowOnceFunc bool, budget *ssaflow.SearchBudget) ([]*ssa.MakeClosure, bool) {
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	var result []*ssa.MakeClosure
	var resolve func(ssaflow.ReachingWalk, ssa.Value) bool
	resolve = func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		switch typed := value.(type) {
		case *ssa.MakeClosure:
			result = append(result, typed)
			return true
		case *ssa.Call:
			common := typed.Common()
			if allowOnceFunc && ssaflow.CallMatchesSymbol(common, syncOnceFunc) && len(common.Args) == 1 {
				return walk.Every(common.Args[0], resolve)
			}
		case *ssa.UnOp:
			if stored := heapmodel.NewStorage(budget).StableContent(typed.X, invocation); stored.Proven() {
				return walk.Every(stored.Value, resolve)
			}
		case *ssa.Alloc:
			if stored := heapmodel.NewStorage(budget).StableContent(typed, invocation); stored.Proven() {
				return walk.Every(stored.Value, resolve)
			}
		}
		return false
	}
	if !ssaflow.NewReachingWalk(forms).Within(budget).Every(value, resolve) {
		return nil, false
	}
	return result, len(result) > 0
}
