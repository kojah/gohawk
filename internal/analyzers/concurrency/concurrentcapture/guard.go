package concurrentcapture

import (
	"go/ast"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syncmodel"
	"golang.org/x/tools/go/ssa"
)

type lockGuardProof struct {
	guarded bool
	known   bool
	reason  string
}

// lockGuard asks the broker for complete effects along one worker's ordered
// prefix, then lets the shared region fold exact lock ownership. It never
// treats an unknown helper or conditional execution as proof of no guard.
// Unsupported shapes fall back to the analyzer's conservative syntax policy.
func (evidence captureEvidence) lockGuard(closure *ast.FuncLit, mutation ast.Node) lockGuardProof {
	function := evidence.workers[closure]
	block, ok := captureWorkerBlock(function)
	if !ok {
		return lockGuardProof{reason: "capture-worker-order-unknown"}
	}
	target, ok := mutationInstruction(block, mutation)
	if !ok {
		return lockGuardProof{reason: "capture-mutation-site-unknown"}
	}
	var region syncmodel.LockRegion
	budget := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	for _, instruction := range block.Instrs[:target] {
		switch instruction := instruction.(type) {
		case *ssa.Call:
			summary, available := evidence.provider.ConcurrencyAtCall(instruction, budget)
			if available != summaries.Available || !summary.Complete() {
				// The broker has no complete lock contract. Preserve the prior
				// conservative syntax boundary rather than treating missing
				// effects as proof that the mutation is unguarded.
				return lockGuardProof{reason: "capture-helper-effects-unknown"}
			}
			region.Apply(summary.Operations)
		case *ssa.Defer:
			// Registration does not execute the deferred release here.
		case *ssa.Go, *ssa.Select, *ssa.RunDefers:
			return lockGuardProof{reason: "capture-worker-order-unknown"}
		}
	}
	held, certain := region.Held()
	if !certain {
		return lockGuardProof{guarded: true, known: true, reason: "capture-lock-identity-unknown"}
	}
	if held {
		return lockGuardProof{guarded: true, known: true, reason: "capture-lock-held"}
	}
	return lockGuardProof{known: true, reason: "capture-no-lock-held"}
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
