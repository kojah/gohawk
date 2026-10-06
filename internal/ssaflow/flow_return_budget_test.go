package ssaflow

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestNormalReturnProofAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returns", `package returns
import "os"
func normal() {}
func forever() { for {} }
func die() { os.Exit(0) }
func deferred() { defer os.Exit(0) }
func conditional(flag bool) { if flag { defer os.Exit(0) } }
`)
	for _, test := range []struct {
		name      string
		reachable bool
	}{{"normal", true}, {"forever", false}, {"die", false}, {"deferred", false}, {"conditional", true}} {
		function := pkg.Func(test.name)
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		proof := ProveNormalReturnWithin(function.Blocks[0], nil, fresh)
		if !proof.Known() || proof.Proven() != test.reachable || fresh.Exhausted() ||
			(proof.Witness != nil) != test.reachable || NormalReturnReachableWith(function.Blocks[0], nil) != test.reachable {
			t.Fatalf("%s: fresh return proof = %+v", test.name, proof)
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			proof := ProveNormalReturnWithin(function.Blocks[0], nil, cut)
			if proof.Known() || proof.Witness != nil || proof.Reason != proofs.EvidenceBudgetExhausted || !cut.Exhausted() {
				t.Fatal("interrupted reachability cannot prove a return or its absence")
			}
		}
		pool := proofs.NewSearchBudget(0)
		if proof := ProveNormalReturnWithin(function.Blocks[0], nil, pool.Within(proofs.QueryBudget)); proof.Known() || proof.Witness != nil ||
			!pool.Exhausted() {
			t.Fatal("pool cutoff cannot establish normal-return reachability")
		}
	}
	if proof := ProveNormalReturnWithin(nil, nil, proofs.NewSearchBudget(proofs.QueryBudget)); proof.Known() || proof.Reason != proofs.EvidenceUnavailable {
		t.Fatal("missing entry is unavailable, not proof of termination")
	}
}

func TestNormalReturnRejectsInterruptedTerminator(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returns", `package returns
func marker() {}
func subject() { marker() }
`)
	for _, answer := range []bool{false, true} {
		budget := proofs.NewSearchBudget(proofs.QueryBudget)
		called := false
		proof := ProveNormalReturnWithin(pkg.Func("subject").Blocks[0], func(*ssa.Call) bool {
			called = true
			for budget.Spend() {
			}
			return answer
		}, budget)
		if !called || !budget.Exhausted() || proof.Known() || proof.Witness != nil || proof.Reason != proofs.EvidenceBudgetExhausted {
			t.Fatal("an interrupted terminator cannot prove presence or absence of a return")
		}
	}
}
