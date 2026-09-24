package syncgraph

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"golang.org/x/tools/go/ssa"
)

func TestLockRegionTracksExactHeldMutexes(t *testing.T) {
	mutex := new(ssa.Alloc)
	other := new(ssa.Alloc)
	operation := func(kind concurrencyfacts.Kind, resource ssa.Value) concurrencyfacts.Operation {
		return concurrencyfacts.Operation{Kind: kind, Resource: concurrencyfacts.Reference{Value: resource}}
	}
	var region LockRegion
	region.Apply([]concurrencyfacts.Operation{operation(concurrencyfacts.Lock, mutex), operation(concurrencyfacts.Lock, other)})
	if held, known := region.Held(); !held || !known {
		t.Fatalf("two held locks = (%v, %v)", held, known)
	}
	region.Apply([]concurrencyfacts.Operation{operation(concurrencyfacts.Unlock, mutex)})
	if held, known := region.Held(); !held || !known {
		t.Fatalf("one held lock = (%v, %v)", held, known)
	}
	region.Apply([]concurrencyfacts.Operation{operation(concurrencyfacts.Unlock, other)})
	if held, known := region.Held(); held || !known {
		t.Fatalf("released locks = (%v, %v)", held, known)
	}
}

func TestLockRegionDeclinesUncertainRelease(t *testing.T) {
	for _, operation := range []concurrencyfacts.Operation{
		{Kind: concurrencyfacts.Unlock, Resource: concurrencyfacts.Reference{Value: new(ssa.Alloc)}},
		{Kind: concurrencyfacts.Lock, Resource: concurrencyfacts.Reference{Value: new(ssa.Alloc), Indirect: true}},
		{Kind: concurrencyfacts.CondWait},
	} {
		var region LockRegion
		region.Apply([]concurrencyfacts.Operation{operation})
		if _, known := region.Held(); known {
			t.Errorf("uncertain operation %+v produced a known region", operation)
		}
	}
}
