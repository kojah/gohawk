package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestEmbeddedFieldBindingKeepsExactAddressPolicy(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fields", `package fields
import "sync"
type Owner struct { mu sync.Mutex }
func helper(owner *Owner) { owner.mu.Lock(); owner.mu.Unlock() }
func existing(owner *Owner) *sync.Mutex { helper(owner); return &owner.mu }
func fresh() *sync.Mutex { owner := new(Owner); helper(owner); return &owner.mu }
func absent(owner *Owner) { helper(owner) }
func changed(cell **Owner, replacement *Owner) *sync.Mutex {
 first := *cell
 helper(first)
 *cell = replacement
 return &(*cell).mu
}
`)
	for _, test := range []struct {
		name     string
		complete bool
	}{
		{"existing", true},
		{"fresh", true},
		{"absent", false},
		{"changed", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			if len(calls) != 1 {
				t.Fatalf("got %d calls, want helper only", len(calls))
			}
			result := NewEngine().AtCall(calls[0], ssaflow.NewSearchBudget(2000))
			if (result.Reason == ReasonNone) != test.complete {
				t.Fatalf("binding completeness changed: %+v", result)
			}
			if !test.complete {
				return
			}
			if len(result.Operations) != 2 {
				t.Fatalf("lost bound mutex operations: %+v", result)
			}
		})
	}
}
