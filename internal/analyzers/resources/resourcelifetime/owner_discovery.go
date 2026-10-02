package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Owner discovery collects possible local holders through the same storage
// disposition used by the classifier. A complete census proves only that this
// model collected its candidates; it never proves ownership or cleanup.
type resourceOwnerDiscoveryProof struct {
	ssaflow.Proof
	Owners []ssa.Value
	Stores map[*ssa.Store]resourceStorageProof
}

// discoverResourceOwnersWithin commits only a complete census. Interrupted
// candidate collection must not leave partial owners or storage dispositions
// available to the classifier's later flow queries.
func (analysis *resourceAnalysis) discoverResourceOwnersWithin(budget *ssaflow.SearchBudget) resourceOwnerDiscoveryProof {
	proof := proveLocalResourceOwnersWithin(analysis.function, analysis.resource, budget)
	if proof.Proven() {
		analysis.owners = proof.Owners
		analysis.stores = proof.Stores
	}
	return proof
}

func proveLocalResourceOwnersWithin(function *ssa.Function, resource ssa.Value, budget *ssaflow.SearchBudget) resourceOwnerDiscoveryProof {
	var owners []ssa.Value
	var stores map[*ssa.Store]resourceStorageProof
	unknown := resourceOwnerDiscoveryProof{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}}
	if !budget.Spend() {
		return unknown
	}
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		stored := proveResourceStorage(instruction, resource, budget)
		if stored.Reason == resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
			return unknown
		}
		// Completed dispositions are independent of flow state and can feed
		// the classifier's existing cache, avoiding another storage query.
		if store, ok := instruction.(*ssa.Store); ok {
			if stores == nil {
				stores = make(map[*ssa.Store]resourceStorageProof)
			}
			stores[store] = stored
		}
		// Unknown indirect destinations retain the previous possible-holder
		// policy. Only interrupted evidence prevents completing the census.
		if stored.Owner == nil || stored.Proven() {
			continue
		}
		known := heapmodel.MayAliasAnyWithin(stored.Owner, owners, budget)
		if resourceFlowExhausted(budget) {
			return unknown
		}
		if !known {
			owners = append(owners, stored.Owner)
		}
	}
	if resourceFlowExhausted(budget) {
		return unknown
	}
	return resourceOwnerDiscoveryProof{
		Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk}, Owners: owners, Stores: stores,
	}
}
