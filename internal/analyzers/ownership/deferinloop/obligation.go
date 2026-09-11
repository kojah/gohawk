package deferinloop

import (
	"go/token"
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
		if owned && len(cleanup) > 0 && valueDerivesFrom(target, ssaflow.CallResult(call, result), call.Parent()) {
			return cleanup, true
		}
	}
	return nil, false
}

func resultDerivesToTarget(call *ssa.Call, target ssa.Value) bool {
	results := call.Common().Signature().Results()
	if results.Len() == 1 {
		return valueDerivesFrom(target, ssaflow.CallResult(call, -1), call.Parent())
	}
	for index := range results.Len() {
		if valueDerivesFrom(target, ssaflow.CallResult(call, index), call.Parent()) {
			return true
		}
	}
	return false
}

// Composite literals often store an acquired value through one field/index
// address and load it for the defer through an equivalent sibling address.
// The shared derivation walk handles direct values; this local policy adds the
// one store-to-load bridge needed to identify that same loop-local resource.
func valueDerivesFrom(value, source ssa.Value, function *ssa.Function) bool {
	if ssaflow.ValueDerivesFrom(value, source, map[ssa.Value]bool{}) {
		return true
	}
	load, ok := value.(*ssa.UnOp)
	if !ok {
		return false
	}
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](function) {
		if sameStorageAddress(store.Addr, load.X) && ssaflow.ValueDerivesFrom(store.Val, source, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

// The cleanup and acquisition may independently reload the same field or
// constant index. ProveIdentity compares those access paths without equating
// the selected resource with the aggregate that contains it.
func sameObligationValue(left, right ssa.Value) bool {
	if ssaflow.SameValue(left, right) {
		return true
	}
	leftLoad, leftOK := left.(*ssa.UnOp)
	rightLoad, rightOK := right.(*ssa.UnOp)
	return leftOK && rightOK && leftLoad.Op == token.MUL && rightLoad.Op == token.MUL &&
		sameStorageAddress(leftLoad.X, rightLoad.X)
}

func sameStorageAddress(left, right ssa.Value) bool {
	if ssaflow.SameValue(left, right) {
		return true
	}
	leftRoot, leftOK := storageAddressRoot(left)
	rightRoot, rightOK := storageAddressRoot(right)
	if !leftOK || !rightOK || !storageBasesMatch(leftRoot, rightRoot) {
		return false
	}
	return ssaflow.ProveIdentity(
		ssaflow.AccessPath{Value: left, Root: leftRoot},
		ssaflow.AccessPath{Value: right, Root: rightRoot},
	).Proven()
}

func storageAddressRoot(value ssa.Value) (ssa.Value, bool) {
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		return typed.X, true
	case *ssa.IndexAddr:
		return typed.X, true
	default:
		return nil, false
	}
}

func storageBasesMatch(left, right ssa.Value) bool {
	if ssaflow.SameValue(left, right) ||
		ssaflow.ValueDerivesFrom(left, right, map[ssa.Value]bool{}) ||
		ssaflow.ValueDerivesFrom(right, left, map[ssa.Value]bool{}) {
		return true
	}
	if sliced, ok := left.(*ssa.Slice); ok && ssaflow.SameValue(sliced.X, right) {
		return true
	}
	if sliced, ok := right.(*ssa.Slice); ok && ssaflow.SameValue(left, sliced.X) {
		return true
	}
	return false
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
