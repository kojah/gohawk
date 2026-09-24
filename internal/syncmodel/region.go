package syncmodel

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"golang.org/x/tools/go/ssa"
)

// LockRegion folds a complete ordered prefix of synchronization effects.
// It answers whether any exact mutex is held at a point; callers decide what
// that evidence means for their own diagnostic. Indirect locks, unmatched
// unlocks, and condition waits make the result unknown.
type LockRegion struct {
	held    map[ssa.Value]bool
	unknown bool
}

// Apply consumes effects in execution order. It never infers a missing effect
// from an empty sequence: the caller must have established prefix completeness.
func (region *LockRegion) Apply(operations []concurrencyfacts.Operation) {
	for _, operation := range operations {
		if operation.Resource.Projection.Depth != 0 {
			region.unknown = true
			continue
		}
		switch operation.Kind {
		case concurrencyfacts.Lock:
			if operation.Resource.Indirect || operation.Resource.Value == nil {
				region.unknown = true
				continue
			}
			if region.held == nil {
				region.held = make(map[ssa.Value]bool)
			}
			region.held[operation.Resource.Value] = true
		case concurrencyfacts.Unlock:
			if operation.Resource.Indirect || operation.Resource.Value == nil || !region.held[operation.Resource.Value] {
				region.unknown = true
				continue
			}
			delete(region.held, operation.Resource.Value)
		case concurrencyfacts.CondWait, concurrencyfacts.ReadLock, concurrencyfacts.ReadUnlock:
			// Wait temporarily releases a Locker. Read locking is not an
			// exclusive guard either; this query deliberately declines both.
			region.unknown = true
		default:
			// Other synchronization effects do not change mutex ownership.
		}
	}
}

// Held reports whether an exact mutex is held and whether the prefix was
// sufficiently modeled to decide that. Unknown is not evidence of no lock.
func (region *LockRegion) Held() (held bool, known bool) {
	return len(region.held) != 0, !region.unknown
}
