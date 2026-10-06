package resultfacts

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
)

func TestTerminationReachabilityCutoffDoesNotCache(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", `package results
import "os"
func Die() { os.Exit(0) }
func Loop() { for {} }
func Returns() {}
func Deferred() { defer os.Exit(0) }
func Conditional(flag bool) { if flag { defer os.Exit(0) } }
`)
	for _, test := range []struct {
		name  string
		never bool
	}{{"Die", true}, {"Loop", true}, {"Returns", false}} {
		function := pkg.Func(test.name)
		// Lookup and the initial result census can finish, but no allowance
		// remains for the distinct reachability proof.
		limit := 1
		for _, block := range function.Blocks {
			limit += len(block.Instrs)
		}
		engine := NewEngine()
		cut := proofs.NewSearchBudget(limit)
		if got := engine.Function(function, cut); got.Available || got.NeverReturns() || !cut.Exhausted() {
			t.Fatalf("%s admitted an incomplete termination summary: %+v", test.name, got)
		}
		if got := engine.Function(function, proofs.NewSearchBudget(proofs.SummaryBudget)); !got.Available || got.NeverReturns() != test.never {
			t.Fatalf("%s cutoff poisoned fresh inference: %+v", test.name, got)
		}
	}
	for _, name := range []string{"Deferred", "Conditional"} {
		function := pkg.Func(name)
		if function.Recover == nil {
			t.Fatal("expected actual SSA recovery entry for deferred execution")
		}
		if got := NewEngine().Function(function, proofs.NewSearchBudget(proofs.SummaryBudget)); !got.Available || got.NeverReturns() {
			t.Fatal("recovery entry must retain the existing no-termination-claim boundary")
		}
	}
}
