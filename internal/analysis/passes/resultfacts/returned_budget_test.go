package resultfacts

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
)

func TestReturnedParameterCutoffDoesNotCache(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", `package results
func Identity(p *int) *int { return p }
`)
	function := pkg.Func("Identity")
	completed := false
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		engine := NewEngine()
		cut := proofs.NewSearchBudget(limit)
		got := engine.Function(function, cut)
		if got.Available {
			parameter, proven := got.ReturnedParameter(0)
			if !proven || parameter != 0 || cut.Exhausted() {
				t.Fatal("completed identity summary lost its exact relation")
			}
			completed = true
			break
		}
		if _, proven := got.ReturnedParameter(0); proven || !cut.Exhausted() {
			t.Fatal("cutoff admitted identity or failed to record exhaustion")
		}
		fresh := engine.Function(function, proofs.NewSearchBudget(proofs.SummaryBudget))
		parameter, proven := fresh.ReturnedParameter(0)
		if !fresh.Available || !proven || parameter != 0 {
			t.Fatal("cutoff poisoned fresh identity inference")
		}
	}
	if !completed {
		t.Fatal("identity inference never completed")
	}
}
