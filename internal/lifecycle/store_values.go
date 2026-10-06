package lifecycle

import (
	"go/token"
	"iter"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Stored values describe possible aggregate contents, including stores through
// selected addresses and pointer loads. The shared work driver owns cycle
// handling; this package selects the address forms and preserves may polarity.

// StoredInto yields every value stored into address, into a field or element
// selected from it, or through a pointer loaded from it. It is the one walk
// for asking what an aggregate holds; callers supply the question about each
// stored value. No ordering or observation-time identity is promised.
func StoredInto(address ssa.Value) iter.Seq[ssa.Value] {
	return StoredIntoWithin(address, nil)
}

// StoredIntoWithin shares address visits and referrer inspection with budget.
// Cutoff may leave a partial sequence; callers inspect exhaustion before using
// its absence as evidence. Consumer early stopping does not exhaust the budget.
// Graph, callback and allocation costs remain independent. Nil is unbounded.
func StoredIntoWithin(address ssa.Value, budget *proofs.SearchBudget) iter.Seq[ssa.Value] {
	return func(yield func(ssa.Value) bool) {
		ssaflow.WalkStatesWithin([]ssa.Value{address}, func(value ssa.Value) ssa.Value { return value }, func(address ssa.Value) ([]ssa.Value, bool) {
			if address == nil || address.Referrers() == nil {
				return nil, true
			}
			var selections []ssa.Value
			for _, reference := range *address.Referrers() {
				if !budget.Spend() {
					return nil, false
				}
				switch typed := reference.(type) {
				case *ssa.Store:
					if typed.Addr == address && !yield(typed.Val) {
						return nil, false
					}
				case *ssa.FieldAddr, *ssa.IndexAddr:
					selections = append(selections, typed.(ssa.Value))
				case *ssa.UnOp:
					// Follow callback-slot pointers too: *owner.cancel = cancel
					// carries the same possible obligation as owner.cancel = cancel.
					if typed.Op == token.MUL {
						selections = append(selections, typed)
					}
				}
				if budget.Exhausted() || budget.PoolExhausted() {
					return nil, false
				}
			}
			return selections, true
		}, budget)
	}
}
