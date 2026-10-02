package ssaflow

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestReachableConstantBlocksAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "reachablebudget", `package reachablebudget
 func use(){}
 func branch(yes bool){if yes{use()}else{return};use()}
 func loop(yes bool){for yes{use()}}
 `)
	for _, name := range []string{"branch", "loop"} {
		for _, outcome := range []Outcome{OutcomeTrue, OutcomeFalse} {
			fn := pkg.Func(name)
			constants := FixedValues{fn.Params[0]: outcome}
			baseline := ReachableBlocksAssuming(fn, constants)
			for limit := 0; limit <= QueryBudget; limit++ {
				budget := NewSearchBudget(limit)
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
