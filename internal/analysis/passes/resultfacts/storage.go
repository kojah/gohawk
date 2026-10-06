package resultfacts

import (
	"go/token"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Result guarantees consume point-in-time storage evidence, not a second heap
// analysis. Read the stored value without identity-only unboxing: a boxed nil
// pointer is a nonnil interface. The shared reaching fold owns value cycles;
// heapmodel owns mutation, escape, and reaching-write uncertainty.
type storedResultQuery struct {
	engine  *Engine
	budget  *proofs.SearchBudget
	storage *heapmodel.Storage
}

func (query *storedResultQuery) resolve(walk ssaflow.ReachingWalk, value ssa.Value) (Guarantee, bool) {
	// Keep wrapper, phi and revisit work in the same allowance as leaf and
	// storage evidence. Within preserves this fold's shared cycle history when
	// a stored value reenters it; a fresh traversal could lose that boundary.
	return ssaflow.ResolveReachingValue(walk.Within(query.budget), value, query.leaf, func(guarantee Guarantee) Guarantee { return guarantee })
}

func (query *storedResultQuery) leaf(walk ssaflow.ReachingWalk, value ssa.Value) (Guarantee, bool) {
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL {
		if query.storage == nil {
			query.storage = heapmodel.NewStorage(query.budget)
		}
		stored := query.storage.ContentFromWrites(load.X, load)
		if !stored.Proven() || query.budget.Exhausted() {
			return Unknown, false
		}
		return query.resolve(walk, stored.Value)
	}
	guarantee := query.engine.leaf(value, query.budget)
	return guarantee, guarantee != Unknown
}
