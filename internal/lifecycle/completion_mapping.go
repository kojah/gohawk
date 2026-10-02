package lifecycle

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Mapping the caller's target onto a callee's locals. Each mapped local
// says how the callee reaches the target: exactly, as a projection of it,
// as an owner of it, or as a callback bound to it. The mapping is where the
// completion search meets the storage model, so the rules here decide
// what a deferred literal or a helper is credited with.

func (search *completionSearch) mappedLocals(callee completionCallee, target ssa.Value, invocation ssa.Instruction) []mappedLocal {
	var result []mappedLocal
	for binding := range ssaflow.CallBindingsWithin(callee.common, callee.function, callee.closure, search.budget) {
		var local mappedLocal
		var ok bool
		if binding.Captured {
			local, ok = search.capturedLocal(callee, binding.Local, binding.Supplied, target, invocation)
		} else {
			local, ok = search.argumentLocal(binding.Local, binding.Supplied, target, invocation)
		}
		if search.budget.Exhausted() || search.budget.PoolExhausted() {
			return nil
		}
		if ok {
			result = append(result, local)
		}
	}
	// A completed prefix says nothing about an unvisited binding. Never let
	// metadata or mapping cutoff publish locals to the coverage proof.
	if search.budget.Exhausted() || search.budget.PoolExhausted() {
		return nil
	}
	return result
}

// capturedLocal maps a captured variable. A deferred closure observes an
// addressable capture when it runs, after any reassignment that follows the
// defer, so such captures must have exactly one dominating store.
func (search *completionSearch) capturedLocal(
	callee completionCallee,
	free, binding ssa.Value,
	target ssa.Value,
	invocation ssa.Instruction,
) (mappedLocal, bool) {
	if search.exactTarget {
		value := binding
		if cell, ok := binding.(*ssa.Alloc); ok {
			stored := heapmodel.NewStorage(search.budget).StableContent(cell, invocation)
			if !stored.Proven() {
				return mappedLocal{}, false
			}
			value = stored.Value
		}
		return mappedLocal{local: free, supplied: value, kind: localExact}, value == target
	}
	value := ssaflow.CapturedBindingValueWithin(binding, search.budget)
	bindingMatches := heapmodel.CapturedBindingMatchesWithin(binding, target, search.budget)
	if search.budget.Exhausted() || search.budget.PoolExhausted() {
		return mappedLocal{}, false
	}
	// Deferred stability may replace exact with a different alias question;
	// retain the original binding match for the later containment boundary.
	exact := bindingMatches
	if callee.launch == launchDeferred && invocation != nil {
		if cell, ok := binding.(*ssa.Alloc); ok && valueHasDirectStore(cell) {
			// A deferred literal reads its captured cell when the deferred
			// calls run, after every store that follows the defer.
			return search.deferredCellLocal(free, cell, target, invocation)
		}
		stable, ok := deferredBindingValue(binding, target, invocation)
		if !ok {
			return mappedLocal{}, false
		}
		value, exact = stable, heapmodel.MayAlias(stable, target)
	}
	switch {
	case exact:
		return mappedLocal{local: free, supplied: value, kind: localExact}, true
	case search.valueCallsMethod(value, target):
		return mappedLocal{local: free, supplied: value, kind: localCallback}, true
	case heapmodel.ValueDerivesFromWithin(value, target, search.budget):
		// The closure captured a projection of the target, such as a body
		// selected from a response before the literal was created.
		return mappedLocal{local: free, supplied: value, kind: localExact}, true
	case ssaflow.ValueIsAccessPathFromWithin(target, value, search.budget):
		return mappedLocal{local: free, supplied: value, kind: localOwner}, true
	case ssaflow.ValueIsAccessPathFromWithin(target, binding, search.budget):
		// Keep the captured cell as the root on both sides of the field
		// mapping. Peeling just one side would require a may-alias match.
		cell, ok := binding.(*ssa.Alloc)
		if !ok {
			return mappedLocal{}, false
		}
		stored := heapmodel.NewStorage(search.budget).StableContent(cell, invocation)
		return mappedLocal{local: free, supplied: binding, kind: localOwner}, stored.Proven()
	case !bindingMatches && ProveMayContainValueWithin(binding, target, search.budget).Proven():
		// The closure captured an aggregate that stores the target, such as a
		// local closer slice the target was appended to; a lifecycle call on
		// anything selected from that local reaches the target. A cell that
		// held the target directly is decided by the exact rules above.
		return mappedLocal{local: free, supplied: binding, kind: localExact}, true
	}
	return mappedLocal{}, false
}

// deferredCellLocal maps a captured cell by what it holds when the deferred
// calls run, which the points-to graph reads at the function's RunDefers.
// The cell is the target when every object it may hold then is the target
// itself, or nil: a cell cleared after a successful settlement and checked
// by the deferred literal still reaches the target on every path where it
// was not settled, as witness rolls back a transaction:
// https://github.com/transparency-dev/witness/blob/f8056f8fa0d33469be430dac2982e1f3cf745322/persistence/sqlite/sql.go#L199-L207
// The cell contains the target when every object it may hold is an
// aggregate the target was stored into, as rules_img drains a closer slice
// it kept appending to after the defer:
// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/mtree/mtree.go#L110-L128
// A cell that may hold something else when the calls run, or one the graph
// cannot read, is not mapped; a later reassignment is exactly the case a
// store-time reading would get wrong.
func (search *completionSearch) deferredCellLocal(free ssa.Value, cell *ssa.Alloc, target ssa.Value, invocation ssa.Instruction) (mappedLocal, bool) {
	// A captured owner cell holds the object, not its embedded mutex.
	// Prove the cell remains stable, then bind the exact storage path as
	// argumentLocal does. Reassignment keeps this mapping unavailable.
	// https://github.com/basecamp/basecamp-cli/blob/d91fc7b3ae5ee3c54a7fea389f59e791173e647b/internal/connector/queue.go#L264-L290
	stored := heapmodel.NewStorage(search.budget).StableContent(cell, invocation)
	if stored.Proven() {
		if owner := sameValueStorageOwner(target, stored.Value, search.budget); owner != nil {
			return mappedLocal{local: free, supplied: owner, kind: localOwner}, true
		}
	}
	relation, known := heapmodel.DeferredCellRelation(cell, target, invocation)
	if !known {
		return mappedLocal{}, false
	}
	switch relation {
	case heapmodel.DeferredCellExact:
		return mappedLocal{local: free, supplied: target, kind: localExact}, true
	case heapmodel.DeferredCellContains:
		return mappedLocal{local: free, supplied: cell, kind: localExact}, true
	case heapmodel.DeferredCellUnknown:
		if !targetStoredOnPath(cell, target, invocation) {
			return mappedLocal{}, false
		}
		// The merged state says the cell may hold something else, but only
		// on paths that never stored the target: a resource acquired in one
		// branch and closed by a deferred literal after the merge. The
		// target exists only on the paths through its store, and on every
		// one of them the cell still holds it.
		return mappedLocal{local: free, supplied: target, kind: localExact}, true
	}
	return mappedLocal{}, false
}

func (search *completionSearch) argumentLocal(parameter, argument, target ssa.Value, invocation ssa.Instruction) (mappedLocal, bool) {
	if heapmodel.NewStorage(search.budget).Same(argument, target).Proven() {
		return mappedLocal{local: parameter, supplied: argument, kind: localExact}, true
	}
	if search.exactTarget {
		return mappedLocal{local: parameter, supplied: argument, kind: localExact}, argument == target
	}
	// A possible alias must not fall through to aggregate containment and
	// become an exact parameter mapping. Preserve uncertainty for consumers.
	if heapmodel.MayAlias(argument, target) && !heapmodel.DefinitelySameValue(argument, target) {
		*search.incomplete = true
		return mappedLocal{}, false
	}
	switch {
	case ssaflow.ProveIdentityWithin(ssaflow.AccessPath{Value: argument}, ssaflow.AccessPath{Value: target}, search.budget).Proven():
		return mappedLocal{local: parameter, supplied: argument, kind: localExact}, true
	case search.valueCallsMethod(argument, target):
		return mappedLocal{local: parameter, supplied: argument, kind: localCallback}, true
	}
	projection := heapmodel.ProveStrictProjectionPathWithin(argument, target, search.budget)
	if projection.Reason == ssaflow.EvidenceBudgetExhausted {
		search.memo.Incomplete()
		*search.incomplete = true
		return mappedLocal{}, false
	}
	if projection.Proven() {
		return mappedLocal{local: parameter, supplied: argument, kind: localProjection}, true
	}
	if owner := sameValueStorageOwner(target, argument, search.budget); owner != nil {
		// The same storage beneath an owner proven to be the argument: a
		// receiver captured by a closure is spilled to a cell written once,
		// so the lock's owner and the helper's argument are two loads of
		// one value. centrifuge-go locks s.mu and hands s to a helper that
		// unlocks it, with s captured by the function's closures:
		// https://github.com/centrifugal/centrifuge-go/blob/080126041ccc71654718bd0601b920ff8b22a8bf/subscription.go#L1156-L1183
		return mappedLocal{local: parameter, supplied: owner, kind: localOwner}, true
	}
	switch {
	case ProveMayContainValueWithin(argument, target, search.budget).Proven():
		stored := heapmodel.ProveStoredPathWithin(argument, target, invocation, search.budget)
		if stored.Reason == ssaflow.EvidenceBudgetExhausted {
			search.memo.Incomplete()
			*search.incomplete = true
			return mappedLocal{}, false
		}
		return mappedLocal{local: parameter, supplied: argument, kind: localExact, path: stored.Path}, true
	case ssaflow.ValueIsAccessPathFromWithin(target, argument, search.budget):
		return mappedLocal{local: parameter, supplied: argument, kind: localOwner}, true
	}
	return mappedLocal{}, false
}

// sameValueStorageOwner returns the root of target's static field or element
// path when that root is argument or proven to be the same value, and nil
// otherwise. The target is storage beneath the owner, not its stored value;
// a matching cleanup must select the same path, never a sibling field.
// https://github.com/ovn-kubernetes/libovsdb/blob/6acd868996b9393b932a1eeeec1ea4e6c722ebe8/client/client.go#L286-L299
func sameValueStorageOwner(target, argument ssa.Value, budget *ssaflow.SearchBudget) ssa.Value { //nolint:ireturn // SSA values keep their concrete forms.
	switch target.(type) {
	case *ssa.FieldAddr, *ssa.IndexAddr:
		if ssaflow.ValueIsAccessPathFromWithin(target, argument, budget) {
			return argument
		}
	default:
		return nil
	}
	root := target
	for {
		if !budget.Spend() {
			return nil
		}
		switch address := root.(type) {
		case *ssa.FieldAddr:
			root = address.X
			continue
		case *ssa.IndexAddr:
			root = address.X
			continue
		}
		break
	}
	if root == target || !ssaflow.ValueIsAccessPathFromWithin(target, root, budget) ||
		root != argument && !heapmodel.DefinitelySameValue(root, argument) {
		return nil
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil
	}
	return root
}

// receiverProof keeps a possible aggregate cleanup separate from an exact
// receiver match. Only the latter can supply a must-complete action.
type receiverProof struct {
	ssaflow.Proof
	Possible bool
}

// receives proves whether a call receiver stands for the caller's target.
func (local mappedLocal) receives(receiver, target ssa.Value, budget *ssaflow.SearchBudget) receiverProof {
	if receiver == nil {
		return receiverMatchProof(false)
	}
	switch local.kind {
	case localExact:
		if len(local.path) > 0 {
			// A field cleanup must select the target's exact path from the
			// original aggregate. Possible derivation also sees overwritten
			// spill contents, so it cannot rescue an unavailable path. An
			// empty proven path still permits the aggregate's own method.
			actual, ok := heapmodel.AccessPathFromParameterWithin(receiver, local.local, budget)
			if !ok {
				// Possible cleanup remains uncertainty, including a loop's
				// dynamic element. It must never become a must-complete action.
				if heapmodel.ValueDerivesFromWithin(receiver, local.local, budget) {
					return receiverProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}, Possible: true}
				}
				return receiverMatchProof(false)
			}
			if len(actual) > 0 {
				return receiverMatchProof(ssaflow.JoinAccessPath(actual) == ssaflow.JoinAccessPath(local.path))
			}
		}
		return receiverMatchProof(heapmodel.ValueDerivesFromWithin(receiver, local.local, budget))
	case localProjection:
		return receiverMatchProof(exactCleanupReceiver(receiver, local.local, budget))
	case localOwner:
		// The callee closes the path beneath its local that mirrors the
		// target's path beneath the supplied owner, such as resp.Body from a
		// captured resp.
		return receiverProof{Proof: ssaflow.ProveIdentityWithin(ssaflow.AccessPath{Value: receiver, Root: local.local},
			ssaflow.AccessPath{Value: target, Root: local.supplied}, budget).Proof}
	case localCallback:
	}
	return receiverMatchProof(false)
}

func receiverMatchProof(matches bool) receiverProof {
	state, reason := ssaflow.EvidenceDisproven, ssaflow.EvidenceUnavailable
	if matches {
		state, reason = ssaflow.EvidenceProven, ssaflow.EvidenceCalledCompletion
	}
	return receiverProof{Proof: ssaflow.Proof{State: state, Reason: reason}}
}

func exactCleanupReceiver(receiver, parameter ssa.Value, budget *ssaflow.SearchBudget) bool {
	if receiver == nil || parameter == nil || !budget.Spend() {
		return false
	}
	if inner, ok := ssaflow.UnwrapTransparentValue(
		receiver, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return exactCleanupReceiver(inner, parameter, budget)
	}
	return receiver == parameter
}
