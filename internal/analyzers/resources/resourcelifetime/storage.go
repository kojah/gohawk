package resourcelifetime

import (
	"go/token"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Storage disposition resolves the stored resource and its destination once.
// A local aggregate carrying a destination address is not that destination;
// unknown identity cannot establish transfer or an abandoned resource.

type resourceStorageProof struct {
	resourceProof
	Owner ssa.Value
}

func (analysis *resourceAnalysis) resourceStorage(instruction ssa.Instruction) resourceStorageProof {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return resourceStorageProof{resourceProof: resourceProof{State: proofs.EvidenceDisproven}}
	}
	if proof, known := analysis.stores[store]; known {
		return proof
	}
	proof := proveResourceStorage(store, analysis.resource, analysis.budget(proofs.SummaryBudget))
	// An interrupted answer cannot poison a later query with fresh allowance.
	if proof.State == proofs.EvidenceUnknown && proof.Reason == resourceReasonBudgetExhausted {
		return proof
	}
	if analysis.stores == nil {
		analysis.stores = make(map[*ssa.Store]resourceStorageProof)
	}
	analysis.stores[store] = proof
	return proof
}

// proveResourceStorage distinguishes the destination from the local aggregate
// that holds its address. An unresolved pointer load consumes the resource
// opaquely; it is neither a local owner nor guaranteed cleanup. Value derivation
// and containment share budget; destination-origin and graph internals remain
// independent costs. Default owner collection keeps its existing allowance.
// https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/internal/admin/repository/sql_store.go#L73-L99
func proveResourceStorage(instruction ssa.Instruction, resource ssa.Value, budget *proofs.SearchBudget) resourceStorageProof {
	missing := resourceStorageProof{resourceProof: resourceProof{State: proofs.EvidenceDisproven}}
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return missing
	}
	derived := heapmodel.ValueDerivesFromWithin(store.Val, resource, budget)
	if !derived && !resourceFlowExhausted(budget) {
		contains := lifecycle.ProveMayContainValueWithin(store.Val, resource, budget)
		derived = contains.Proven()
	}
	if resourceFlowExhausted(budget) {
		return resourceStorageProof{resourceProof: resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}}
	}
	if !derived {
		return missing
	}
	owner := store.Addr
	if field, ok := store.Addr.(*ssa.FieldAddr); ok {
		owner = field.X
	}
	return proveStorageDestination(store, owner, budget)
}

func proveStorageDestination(store *ssa.Store, owner ssa.Value, budget *proofs.SearchBudget) resourceStorageProof {
	proof := resourceStorageProof{resourceProof: resourceProof{State: proofs.EvidenceDisproven}, Owner: owner}
	if !budget.Spend() {
		proof.State, proof.Reason = proofs.EvidenceUnknown, resourceReasonBudgetExhausted
		return proof
	}
	if ssaflow.ExternallyOwnedValue(owner) {
		proof.State, proof.Reason = proofs.EvidenceProven, resourceReasonSettled
		return proof
	}
	// A store through a pointer the caller supplied lands in caller-owned
	// storage. Preserve the original owner for local cleanup lookup as well.
	// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/validate/layer-presence/flags.go#L83-L94
	load, indirect := store.Addr.(*ssa.UnOp)
	if !indirect || load.Op != token.MUL {
		return proof
	}
	if !budget.Spend() {
		proof.State, proof.Reason = proofs.EvidenceUnknown, resourceReasonBudgetExhausted
		return proof
	}
	object, known := heapmodel.ExclusiveAt(store.Addr, store)
	if !known {
		proof.State, proof.Reason = proofs.EvidenceUnknown, resourceReasonIndirectDestinationUnknown
	} else if !object.Local {
		proof.State, proof.Reason = proofs.EvidenceProven, resourceReasonSettled
	}
	return proof
}
