package deferinloop

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

type deferObligation struct {
	target  ssa.Value
	cleanup []string
}

type lockContract struct {
	acquire syntax.Symbol
	release syntax.Symbol
}

// Locks do not come from constructor results, so their obligation is proven
// from an exact standard-library acquire/release pair on the same SSA value.
var lockContracts = []lockContract{
	{
		acquire: syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Lock"}),
		release: syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Unlock"}),
	},
	{
		acquire: syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Lock"}),
		release: syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Unlock"}),
	},
	{
		acquire: syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RLock"}),
		release: syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RUnlock"}),
	},
}

// deferredObligation flips the old name heuristic's burden of proof. A method
// shaped like Close is a candidate only after an acquisition in this loop
// iteration or an inferred owned-result contract establishes a real lifetime.
func deferredObligation(
	evidence *lifecyclefacts.LifecycleEvidence,
	deferred *ssa.Defer,
) (deferObligation, bool) {
	common := deferred.Common()
	target := ssaflow.CallReceiver(common)
	if target == nil {
		return deferObligation{}, false
	}
	for _, contract := range lockContracts {
		if contract.release.MatchesMethod(ssaflow.CallName(common), target.Type()) &&
			lockAcquiredBeforeDefer(deferred, target, contract.acquire) {
			return deferObligation{target: target, cleanup: []string{ssaflow.CallName(common)}}, true
		}
	}
	cleanup, acquired := resourceAcquiredBeforeDefer(evidence, deferred, target)
	if acquired && slices.Contains(cleanup, ssaflow.CallName(common)) {
		return deferObligation{target: target, cleanup: cleanup}, true
	}
	return deferObligation{}, false
}

// resourceAcquiredBeforeDefer accepts a call result only when the result's
// concrete type is in the shared lifecycle vocabulary, or the callee's
// lifecycle facts prove that its returned aggregate owns a resource.
func resourceAcquiredBeforeDefer(
	evidence *lifecyclefacts.LifecycleEvidence,
	deferred *ssa.Defer,
	target ssa.Value,
) ([]string, bool) {
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](deferred.Parent()) {
		if !acquisitionRepeatsBeforeDefer(call, deferred) {
			continue
		}
		if resultDerivesToTarget(call, target) {
			if cleanup, known := lifecyclefacts.ResourceCleanup(target.Type()); known {
				return cleanup, true
			}
		}
		cleanup, result, owned := evidence.OwnedResult(call)
		if owned && len(cleanup) > 0 && valueDerivesFrom(target, ssaflow.CallResult(call, result)) {
			return cleanup, true
		}
	}
	return nil, false
}

func resultDerivesToTarget(call *ssa.Call, target ssa.Value) bool {
	results := call.Common().Signature().Results()
	if results.Len() == 1 {
		return valueDerivesFrom(target, ssaflow.CallResult(call, -1))
	}
	for index := range results.Len() {
		if valueDerivesFrom(target, ssaflow.CallResult(call, index)) {
			return true
		}
	}
	return false
}

// Resolve each load at its execution point before relating the selected
// resource to its acquisition. Historical writes are not current contents.
func valueDerivesFrom(value, source ssa.Value) bool {
	resolved := ssaflow.NewStorage(ssaflow.NewSearchBudget(1000)).Resolve(value)
	return resolved.Proven() && ssaflow.ValueDerivesFrom(resolved.Value, source, map[ssa.Value]bool{})
}

// Reloading the same address only identifies the same obligation when its
// contents still agree. The storage query owns that temporal distinction.
func sameObligationValue(left, right ssa.Value) bool {
	return ssaflow.NewStorage(ssaflow.NewSearchBudget(1000)).Same(left, right).Proven()
}

// Dominance proves acquisition precedes the defer on this path; reachability
// back to the acquisition block proves it belongs to the repeating region,
// rather than being a function-entry value reused by a later loop.
func acquisitionRepeatsBeforeDefer(acquisition, deferred ssa.Instruction) bool {
	return ssaflow.InstructionDominates(acquisition, deferred) &&
		ssaflow.BlockReachable(deferred.Block(), acquisition.Block())
}

func lockAcquiredBeforeDefer(deferred *ssa.Defer, target ssa.Value, acquire syntax.Symbol) bool {
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](deferred.Parent()) {
		receiver := ssaflow.CallReceiver(call.Common())
		if acquisitionRepeatsBeforeDefer(call, deferred) && receiver != nil &&
			acquire.MatchesMethod(ssaflow.CallName(call.Common()), receiver.Type()) &&
			sameObligationValue(receiver, target) {
			return true
		}
	}
	return false
}
