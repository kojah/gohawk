package resourcemodel

import (
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Relation identifies an exact resource beneath a caller-visible owner at
// one observation point. Containment alone does not claim that the owner is
// responsible for cleanup; a lifecycle proof must establish any transfer.
type Relation struct {
	owner    ssa.Value
	resource ssa.Value
	path     []string
}

// Path returns the field/element route beneath Owner. An empty path means
// Owner and Resource are the same object.
func (relation Relation) Path() []string { return slices.Clone(relation.path) }

// At reports whether the relationship names exactly this access path.
func (relation Relation) At(path []string) bool { return slices.Equal(relation.path, path) }

// RelationProof carries the exact owner-to-resource path established by the
// heap/storage model, or an unknown outcome when that path cannot be named.
type RelationProof struct {
	ssaflow.Proof
	Relation Relation
}

// ProveRelation resolves a direct identity or an exact field/element path
// from owner to resource at observation. It reuses the existing heap model
// through lifecycle; no separate resource points-to graph is maintained.
func ProveRelation(owner, resource ssa.Value, observation ssa.Instruction, budget *ssaflow.SearchBudget) RelationProof {
	unknown := RelationProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	if owner == nil || resource == nil || observation == nil || budget == nil {
		return unknown
	}
	if !budget.Spend() {
		return RelationProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}}
	}
	same := heapmodel.NewStorage(budget).Same(owner, resource)
	if same.Reason == ssaflow.EvidenceBudgetExhausted || budget.Exhausted() || budget.PoolExhausted() {
		return RelationProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}}
	}
	if same.Proven() {
		return RelationProof{
			Proof:    ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceSameAccessPath},
			Relation: Relation{owner: owner, resource: resource},
		}
	}
	path := heapmodel.ProveStoredPathWithin(owner, resource, observation, budget)
	if !path.Proven() {
		return RelationProof{Proof: path.Proof}
	}
	return RelationProof{
		Proof:    ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceSameAccessPath},
		Relation: Relation{owner: owner, resource: resource, path: slices.Clone(path.Path)},
	}
}
