package concurrentcapture

import (
	"go/ast"
	"go/types"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/summaries"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type lockGuardProof struct {
	guarded bool
	known   bool
	reason  captureReason
}

// mutationProof decides whether the selected repeated write remains reportable
// after the analyzer's guard evidence. Proven means the guard policy permits a
// diagnostic; unknown suppresses it without establishing race freedom.
type mutationProof struct {
	state  proofs.EvidenceState
	reason captureReason
}

func (evidence captureEvidence) proveMutation(
	pass *analysis.Pass, closure *ast.FuncLit, mutation ast.Node, varying []types.Object, fallbackLock bool,
) mutationProof {
	guard := evidence.lockGuard(closure, mutation)
	// A held lock is evidence of possible serialization, not proof that
	// every worker uses the same lock. Unknown effects retain the older
	// syntax fallback rather than claiming the write is unguarded.
	switch {
	case guard.known && guard.guarded:
		return mutationProof{proofs.EvidenceUnknown, guard.reason}
	case !guard.known && fallbackLock:
		return mutationProof{proofs.EvidenceUnknown, reasonLockFallbackUnknown}
	case mutationHasWorkerGuard(pass, closure, mutation, varying):
		return mutationProof{proofs.EvidenceUnknown, reasonWorkerGuardUnknown}
	case mutationHasChannelGuard(pass, closure, mutation):
		return mutationProof{proofs.EvidenceUnknown, reasonChannelGuardUnknown}
	default:
		return mutationProof{proofs.EvidenceProven, reasonUnguardedWrite}
	}
}

// lockGuard asks the broker for complete effects along one worker's ordered
// prefix, then folds exact lock ownership over it. It never
// treats an unknown helper or conditional execution as proof of no guard.
// Unsupported shapes fall back to the analyzer's conservative syntax policy.
func (evidence captureEvidence) lockGuard(closure *ast.FuncLit, mutation ast.Node) lockGuardProof {
	function := evidence.workers[closure]
	block, ok := captureWorkerBlock(function)
	if !ok {
		return lockGuardProof{reason: reasonWorkerOrderUnknown}
	}
	target, ok := mutationInstruction(block, mutation)
	if !ok {
		return lockGuardProof{reason: reasonMutationSiteUnknown}
	}
	var region lockRegion
	budget := proofs.NewSearchBudget(proofs.SummaryBudget)
	for _, instruction := range block.Instrs[:target] {
		switch instruction := instruction.(type) {
		case *ssa.Call:
			summary, available := evidence.provider.ConcurrencyAtCall(instruction, budget)
			if available != summaries.Available || !summary.Complete() {
				// The broker has no complete lock contract. Preserve the prior
				// conservative syntax boundary rather than treating missing
				// effects as proof that the mutation is unguarded.
				return lockGuardProof{reason: reasonHelperEffectsUnknown}
			}
			region.apply(summary.Operations)
		case *ssa.Defer:
			// Registration does not execute the deferred release here.
		case *ssa.Go, *ssa.Select, *ssa.RunDefers:
			return lockGuardProof{reason: reasonWorkerOrderUnknown}
		}
	}
	held, certain := region.heldState()
	if !certain {
		return lockGuardProof{guarded: true, known: true, reason: reasonLockIdentityUnknown}
	}
	if held {
		return lockGuardProof{guarded: true, known: true, reason: reasonLockHeld}
	}
	return lockGuardProof{known: true, reason: reasonNoLockHeld}
}

func captureWorkerBlock(function *ssa.Function) (*ssa.BasicBlock, bool) {
	if function == nil || len(function.Blocks) == 0 {
		return nil, false
	}
	if len(function.Blocks) == 1 {
		return function.Blocks[0], true
	}
	if len(function.Blocks) != 2 || function.Recover != function.Blocks[1] || len(function.Blocks[0].Succs) != 0 {
		return nil, false
	}
	return function.Blocks[0], true
}

func mutationInstruction(block *ssa.BasicBlock, mutation ast.Node) (int, bool) {
	target := -1
	for index, instruction := range block.Instrs {
		switch instruction.(type) {
		case *ssa.Store, *ssa.MapUpdate:
		default:
			continue
		}
		if position := instruction.Pos(); position < mutation.Pos() || position > mutation.End() {
			continue
		}
		if target >= 0 {
			return 0, false
		}
		target = index
	}
	return target, target >= 0
}
