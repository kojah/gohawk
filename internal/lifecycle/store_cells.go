package lifecycle

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

var syncOnceFunc = syntax.PackageFunction("sync", "OnceFunc")

// Deferred bindings are observed when the deferred callee runs, after any
// assignment that follows the defer. These helpers recover the one value an
// addressable capture or stored callback holds at that point. Stable storage
// queries require agreeing incoming writes and no later mutation; specialized
// target-relative proofs also account for conditional acquisition paths.

// deferredBindingValue recovers the value a non-cell binding stands for
// when the deferred callee runs. A captured cell is read by the points-to
// graph instead; see deferredCellLocal.
// Stable lookup keeps the default storage child cap and preserves its cutoff
// reason so completion can invalidate an enclosing memo answer.
func deferredBindingValue(binding, target ssa.Value, invocation ssa.Instruction, budget *ssaflow.SearchBudget) heapmodel.StoredValue {
	if !budget.Spend() {
		return heapmodel.StoredValue{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}}
	}
	if heapmodel.MayAlias(binding, target) || ssaflow.ValueIsAccessPathFromWithin(target, binding, budget) {
		return heapmodel.StoredValue{Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceCapturedByClosure}, Value: binding}
	}
	return heapmodel.NewStorage(budget.Within(ssaflow.QueryBudget)).StableContent(binding, invocation)
}

func valueHasDirectStore(value ssa.Value, budget *ssaflow.SearchBudget) bool {
	if value == nil || value.Referrers() == nil {
		return false
	}
	for _, reference := range *value.Referrers() {
		if !budget.Spend() {
			return false
		}
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == value {
			return true
		}
	}
	return false
}

// targetStoredOnPath reports whether a store of the target itself reaches the
// observation with no other store between them and none after. The target is
// defined where it is stored, so every path on which it is live passes that
// store; a resource acquired in one branch of an if/else and closed by a
// deferred literal after the merge is settled this way even though the store
// does not dominate the defer. traefikoidc assigns a response in either a
// retry callback or a direct call before deferring its close:
// https://github.com/lukaszraczylo/traefikoidc/blob/61e60733a5be38428dee42eed626490f9609dad6/token_introspection.go#L84-L115
func targetStoredOnPath(address, target ssa.Value, observation ssa.Instruction, budget *ssaflow.SearchBudget) ssaflow.Proof {
	if address == nil || address.Referrers() == nil {
		return ssaflow.Proof{Reason: ssaflow.EvidenceUnavailable}
	}
	var stores []*ssa.Store
	for _, reference := range *address.Referrers() {
		if !budget.Spend() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address {
			stores = append(stores, store)
		}
	}
	for _, candidate := range stores {
		if !budget.Spend() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		matches := heapmodel.MayAlias(candidate.Val, target)
		reaches := matches && ssaflow.InstructionMayFollowWithin(candidate, observation, budget)
		if budget.Exhausted() || budget.PoolExhausted() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if !reaches {
			continue
		}
		intervening := proveInterveningStore(address, candidate, observation, stores, budget)
		if intervening.State == ssaflow.EvidenceUnknown {
			return intervening
		}
		if !intervening.Proven() {
			return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStoredInEnclosingScope}
		}
	}
	return ssaflow.Proof{Reason: ssaflow.EvidenceUnavailable}
}

// proveInterveningStore checks the completed census before a target-relative
// witness can be credited. A shortened absence search remains unknown.
func proveInterveningStore(
	address ssa.Value, candidate *ssa.Store, observation ssa.Instruction, stores []*ssa.Store, budget *ssaflow.SearchBudget,
) ssaflow.Proof {
	for _, other := range stores {
		if !budget.Spend() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if other == candidate {
			continue
		}
		follows := heapmodel.StoreMayFollowWithin(address, observation, other, budget)
		between := !follows && ssaflow.InstructionMayFollowWithin(candidate, other, budget) &&
			ssaflow.InstructionMayFollowWithin(other, observation, budget)
		if budget.Exhausted() || budget.PoolExhausted() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if follows || between {
			return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStorageConflictingWrites}
		}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
}
