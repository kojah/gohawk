package resourcelifetime

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// Presence evidence controls resource activation on one CFG edge. It retains
// the existing possible-derivation policy, sharing traversal and storage work
// with the flow allowance. Cutoff cannot establish an absent acquisition.

// holdsResource reports whether a compared value may be the resource: it
// derives from the resource and its type can hold it. An error returned by a
// helper that was handed the file derives from the file, but no error value
// is the file, so its nil check says nothing about whether the file exists.
func holdsResource(value, resource ssa.Value, budget *proofs.SearchBudget) bool {
	return budget.Spend() && types.AssignableTo(resource.Type(), value.Type()) && heapmodel.ValueDerivesFromWithin(value, resource, budget)
}

// presenceOperand reports whether a nil comparison of value decides whether
// the resource holds anything to release: value is the resource itself, or
// the Body of the net/http response that is the resource. A response whose
// Body is nil has nothing to close, and net/http documents that a response
// returned without error always has a non-nil Body, so a close guarded by
// `resp != nil && resp.Body != nil` covers every feasible path:
// https://github.com/Authula/authula/blob/87a880a2872fae95d749d7a250db0274524fafce/plugins/oauth2/services/base_provider.go#L63-L74
func presenceOperand(value, resource ssa.Value, budget *proofs.SearchBudget) bool {
	if holdsResource(value, resource, budget) {
		return true
	}
	field := lifecyclefacts.ResponseBodyField(value)
	return field != nil && holdsResource(field.X, resource, budget)
}

type resourcePresenceProof struct {
	proofs.Proof
	Present bool
}

func unknownResourcePresence(budget *proofs.SearchBudget) resourcePresenceProof {
	reason := proofs.EvidenceUnavailable
	if resourceFlowExhausted(budget) {
		reason = proofs.EvidenceBudgetExhausted
	}
	return resourcePresenceProof{Proof: proofs.Proof{Reason: reason}}
}

func provenResourcePresence(present bool) resourcePresenceProof {
	return resourcePresenceProof{Proof: proofs.Proof{
		State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA,
	}, Present: present}
}

func proveResourcePresenceBranch(block, predecessor, successor *ssa.BasicBlock, resource ssa.Value, budget *proofs.SearchBudget) resourcePresenceProof {
	if resource == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return unknownResourcePresence(budget)
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return unknownResourcePresence(budget)
	}
	// A comma-ok assertion of the resource's own static type, or of an
	// interface it implements, holds when the asserted value is the
	// resource: the false arm has no owned value to release. moby keeps an
	// io.Reader that may be a file and defers Close under the assertion:
	// https://github.com/moby/moby/blob/3f6733064ea2ea9c00a4a2a9c5c9c5fbd7b7b1d5/daemon/builder/remotecontext/internal/tarsum/tarsum_test.go#L347-L349
	// Saving a short-circuit condition introduces a phi. Only the incoming
	// comparison on this path supplies evidence: an unrelated flag or a phi
	// from an earlier block cannot establish that this resource is absent.
	condition := ssapath.BranchValueWithin(branch.Cond, block, predecessor, budget)
	asserted := assertedResource(condition, resource, budget)
	if resourceFlowExhausted(budget) {
		return unknownResourcePresence(budget)
	}
	if asserted {
		return provenResourcePresence(successor == block.Succs[0])
	}
	comparison, ok := condition.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return unknownResourcePresence(budget)
	}
	comparesResourceToNil := presenceOperand(comparison.X, resource, budget) && ssaflow.DefinitelyNilWithin(comparison.Y, budget) ||
		presenceOperand(comparison.Y, resource, budget) && ssaflow.DefinitelyNilWithin(comparison.X, budget)
	if resourceFlowExhausted(budget) || !comparesResourceToNil {
		return unknownResourcePresence(budget)
	}
	trueBranch := successor == block.Succs[0]
	// On the nil branch there is no owned value to release. This matters when
	// callers defensively close a response whenever net/http returns one, even
	// on an error path:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/codex_utls_transport_test.go#L305-L319
	if comparison.Op == token.NEQ {
		return provenResourcePresence(trueBranch)
	}
	return provenResourcePresence(!trueBranch)
}

// assertedResource reports whether condition is the ok result of a comma-ok
// type assertion whose operand resolves to the resource and whose asserted
// type the resource's static type satisfies, so the assertion succeeds.
func assertedResource(condition, resource ssa.Value, budget *proofs.SearchBudget) bool {
	okResult, ok := condition.(*ssa.Extract)
	if !ok || okResult.Index != 1 {
		return false
	}
	assertion, ok := okResult.Tuple.(*ssa.TypeAssert)
	if !ok || !assertion.CommaOk {
		return false
	}
	// The asserted operand is usually a load of the cell the resource was
	// stored into, which other paths may have written too; possible
	// derivation suffices, because the rule only ever removes an
	// obligation from the arm where the assertion failed.
	held := heapmodel.NewStorage(budget).Same(assertion.X, resource).Proven() ||
		heapmodel.ValueDerivesFromWithin(assertion.X, resource, budget)
	return held && !resourceFlowExhausted(budget) && budget.Spend() && types.AssignableTo(resource.Type(), assertion.AssertedType)
}

// Unconditional result guarantees refine only feasible edges. They do not
// create acquisition contracts, infer cleanup, or replace error/owner relations.
// Unknown results retain the existing paths and their reporting policy.
func (analysis *resourceAnalysis) successorPolicy() ssapath.SuccessorPolicy {
	if analysis.summaries == nil {
		return ssapath.SuccessorPolicy{}
	}
	return ssapath.SuccessorPolicy{Feasible: func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
		return analysis.summaries.FeasibleSuccessors(block, predecessor, analysis.budget(2000))
	}}
}

// acquisitionReachable reports whether some feasible path from the entry
// reaches the acquisition. The obligation walk prunes edges only after the
// acquisition; this asks the same question of the path before it. A helper
// whose result is nil whenever its error is non-nil makes a retry guarded by
// res != nil after the error check unreachable, and a resource no feasible
// path acquires owes nothing. Only a proven prune removes a path: unknown
// results keep every edge, and an exhausted budget keeps the acquisition.
// https://github.com/gan-of-culture/get-sauce/blob/d726f56e7424bde4ff31e5329f37018343956103/request/request.go#L308-L318
func (analysis *resourceAnalysis) acquisitionReachable(budget *proofs.SearchBudget) bool {
	type position struct{ block, predecessor *ssa.BasicBlock }
	type positionKey struct{ block, predecessor int }
	target := analysis.acquisition.Block()
	reached := false
	key := func(at position) positionKey {
		predecessor := -1
		if at.predecessor != nil {
			predecessor = at.predecessor.Index
		}
		return positionKey{block: at.block.Index, predecessor: predecessor}
	}
	cfg.WalkStatesWithin([]position{{block: analysis.function.Blocks[0]}}, key, func(at position) ([]position, bool) {
		if at.block == target || !budget.Spend() {
			reached = true
			return nil, false
		}
		var next []position
		for _, successor := range analysis.successorPolicy().SuccessorsWithin(at.block, at.predecessor, budget) {
			next = append(next, position{block: successor, predecessor: at.block})
		}
		return next, true
	}, budget)
	return reached || resourceFlowExhausted(budget)
}
