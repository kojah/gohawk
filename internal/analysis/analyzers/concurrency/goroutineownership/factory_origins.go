package goroutineownership

import (
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Factory-origin evidence distinguishes uncertain completion handles from an
// external owner's exact transfer. Value folds and storage leaves share an
// allowance; incomplete origin evidence stops the lifecycle decision as unknown.

func (analysis *spawnAnalysis) completionHandleProof() (GoroutineProof, bool) {
	for _, tracked := range analysis.tracked {
		budget := analysis.budget()
		if tracked.kind == trackedSignal {
			if proof, decided := analysis.factoryOriginDecision(helperSignalOrigin(tracked.value, analysis.spawn, budget), budget); decided {
				return proof, true
			}
		}
		if tracked.kind != trackedOwner && ssaflow.ExternallyOwnedValue(tracked.value) {
			return GoroutineProof{Outcome: GoroutineTransferred, Reason: reasonCallerOrExternalOwner}, true
		}
		if tracked.kind == trackedGroup {
			if proof, decided := analysis.factoryOriginDecision(opaqueGroupOrigin(tracked.value, budget), budget); decided {
				return proof, true
			}
		}
	}
	return GoroutineProof{}, false
}

func (analysis *spawnAnalysis) factoryOriginDecision(origin proofs.Proof, budget *proofs.SearchBudget) (GoroutineProof, bool) {
	if !origin.Known() {
		return analysis.lifetimeCutoff(budget, queryFactoryOrigin, reasonFactoryOriginBudgetExhausted), true
	}
	if origin.Proven() {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}, true
	}
	return GoroutineProof{}, false
}

func factoryOriginProof(found bool, budget *proofs.SearchBudget) proofs.Proof {
	if budget.Exhausted() {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	if found {
		return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA}
	}
	return proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
}

// A channel supplied by a factory or registry may already have another owner.
// Until its allocation and ownership are established, a local launch cannot
// manufacture an exclusive receive obligation for this caller. This is not a
// claim that an arbitrary factory channel is drained.
// https://github.com/deckarep/golang-set/blob/711c30df0fdf98710a4ca0211e12ef7210967ad3/threadsafe.go#L268-L287
func helperSignalOrigin(value ssa.Value, spawn *ssa.Go, budget *proofs.SearchBudget) proofs.Proof {
	storage := heapmodel.NewStorage(budget)
	var leaf func(ssaflow.ReachingWalk, ssa.Value) bool
	leaf = func(walk ssaflow.ReachingWalk, current ssa.Value) bool {
		if resolved := storage.Resolve(current); resolved.Proven() && resolved.Value != current {
			return walk.Any(resolved.Value, leaf)
		}
		// A returned struct is copied into a local before invoking its pointer
		// method. That copy does not manufacture exclusive ownership of the
		// factory's channel; a companion result may be its actual join handle.
		// https://github.com/tus/tusd/blob/c9d174d0e20c69f24e9785d2f639df4da1c4fdc5/pkg/s3store/s3store_part_producer_test.go#L28-L56
		if _, allocated := current.(*ssa.Alloc); allocated {
			if content := storage.Content(current, spawn); content.Proven() && content.Value != current {
				return walk.Any(content.Value, leaf)
			}
		}
		if result, ok := current.(*ssa.Extract); ok {
			return walk.Any(result.Tuple, leaf)
		}
		_, call := current.(*ssa.Call)
		return call
	}
	return factoryOriginProof(ssaflow.NewReachingWalk(carryForms).Within(budget).Any(value, leaf), budget)
}

// An opaque producer can lend a registry-owned group, not allocate a new one.
// Without its body we cannot assign the join obligation to this invocation.
// This is uncertainty, not proof that the caller or registry actually waits.
// https://github.com/i-love-flamingo/flamingo/blob/79a55d62bb7a1bffe11a4dea1444490b14785879/core/requesttask/filter.go#L28-L60
func opaqueGroupOrigin(value ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	storage := heapmodel.NewStorage(budget)
	var leaf func(ssaflow.ReachingWalk, ssa.Value) bool
	leaf = func(walk ssaflow.ReachingWalk, current ssa.Value) bool {
		if resolved := storage.Resolve(current); resolved.Proven() && resolved.Value != current {
			return walk.Any(resolved.Value, leaf)
		}
		switch typed := current.(type) {
		case *ssa.FieldAddr:
			// An embedded group retains its aggregate's possible registry owner.
			// Resolve loads first: a fresh pointer stored into an owner's group
			// field must not borrow the owner's opaque factory provenance.
			// https://github.com/FDio/govpp/blob/c71484d8c74da940abbd70407b53894fa4c56f01/extras/gomemif/examples/bridge/bridge.go#L50-L100
			return walk.Any(typed.X, leaf)
		case *ssa.Extract:
			return walk.Any(typed.Tuple, leaf)
		case *ssa.Call:
			callee, _ := ssacall.DirectCallee(typed.Common())
			return callee == nil || len(callee.Blocks) == 0
		}
		return false
	}
	return factoryOriginProof(ssaflow.NewReachingWalk(carryForms|ssaflow.TransparentTypeAssert).Within(budget).Any(value, leaf), budget)
}
