package deferinloop

import (
	"slices"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

type deferObligation struct {
	target  ssa.Value
	cleanup []string
	// owner is the response whose Body the target is, or nil. The body's
	// lifetime ends with its response's, so the response's own uses count too.
	owner ssa.Value
}

// values returns the values whose uses decide the obligation.
func (obligation deferObligation) values() []ssa.Value {
	if obligation.owner == nil {
		return []ssa.Value{obligation.target}
	}
	return []ssa.Value{obligation.target, obligation.owner}
}

// closedBy reports whether receiver is the value the obligation's cleanup
// closes: the target itself, or another load of the owning response's Body.
func (obligation deferObligation) closedBy(receiver ssa.Value) bool {
	if sameObligationValue(receiver, obligation.target) {
		return true
	}
	field := lifecyclefacts.ResponseBodyField(receiver)
	return obligation.owner != nil && field != nil && sameObligationValue(field.X, obligation.owner)
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
		obligation := deferObligation{target: target, cleanup: cleanup}
		if field := lifecyclefacts.ResponseBodyField(target); field != nil {
			obligation.owner = field.X
		}
		return obligation, true
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
		if responseBodyAcquired(call, target) {
			return []string{"Close"}, true
		}
		cleanup, result, owned := evidence.OwnedResult(call)
		if owned && len(cleanup) > 0 && valueDerivesFrom(target, ssacall.CallResult(call, result)) {
			return cleanup, true
		}
	}
	return nil, false
}

// A response's resource is its body: a deferred Body.Close is the cleanup of
// the response this iteration acquired. The Body field must still hold what
// the response arrived with; after a local store into it, the defer closes a
// value the acquisition did not produce, so the obligation is not proven.
// https://github.com/gatewayd-io/gatewayd/blob/bb731040d1f5cd848599024d768f2eea50389638/metrics/merger.go#L132-L138
func responseBodyAcquired(call *ssa.Call, target ssa.Value) bool {
	field := lifecyclefacts.ResponseBodyField(target)
	return field != nil && resultDerivesToTarget(call, field.X) && !responseBodyReplaced(call.Parent(), field)
}

func responseBodyReplaced(function *ssa.Function, body *ssa.FieldAddr) bool {
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](function) {
		field, ok := store.Addr.(*ssa.FieldAddr)
		if ok && field.Field == body.Field && heapmodel.MayAlias(field.X, body.X) {
			return true
		}
	}
	return false
}

func resultDerivesToTarget(call *ssa.Call, target ssa.Value) bool {
	results := call.Common().Signature().Results()
	if results.Len() == 1 {
		return valueDerivesFrom(target, ssacall.CallResult(call, -1))
	}
	for index := range results.Len() {
		if valueDerivesFrom(target, ssacall.CallResult(call, index)) {
			return true
		}
	}
	return false
}

// Resolve each load at its execution point before relating the selected
// resource to its acquisition. Historical writes are not current contents.
func valueDerivesFrom(value, source ssa.Value) bool {
	resolved := heapmodel.NewStorage(nil).Resolve(value)
	return resolved.Proven() && heapmodel.ValueDerivesFrom(resolved.Value, source)
}

// Reloading the same address only identifies the same obligation when its
// contents still agree. The storage query owns that temporal distinction.
func sameObligationValue(left, right ssa.Value) bool {
	return heapmodel.NewStorage(nil).Same(left, right).Proven()
}

// Dominance proves acquisition precedes the defer on this path; reachability
// back to the acquisition block proves it belongs to the repeating region,
// rather than being a function-entry value reused by a later loop.
func acquisitionRepeatsBeforeDefer(acquisition, deferred ssa.Instruction) bool {
	return cfg.InstructionDominates(acquisition, deferred) &&
		cfg.BlockReachable(deferred.Block(), acquisition.Block())
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
