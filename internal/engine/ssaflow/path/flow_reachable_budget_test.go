package path

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
)

func TestReachableConstantBlocksAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "reachablebudget", `package reachablebudget
 func use(){}
 func branch(yes bool){if yes{use()}else{return};use()}
 func loop(yes bool){for yes{use()}}
 `)
	for _, name := range []string{"branch", "loop"} {
		for _, outcome := range []ssacall.Outcome{ssacall.OutcomeTrue, ssacall.OutcomeFalse} {
			fn := pkg.Func(name)
			constants := ssacall.FixedValues{fn.Params[0]: outcome}
			baseline := ReachableBlocksAssumingWithin(fn, constants, nil)
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				blocks := ReachableBlocksAssumingWithin(fn, constants, budget)
				if budget.Exhausted() || limit == 0 {
					if blocks != nil {
						t.Fatalf("cut census retained%d blocks", len(blocks))
					}
					continue
				}
				if !slices.Equal(blocks, baseline) {
					t.Fatal("completed census changed discovery order")
				}
				break
			}
		}
	}
}
