package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestMethodCoverageAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "coverage", `package coverage
 func action(){}
 func exact(){action()}
 func conditional(flag bool){if flag{action()}}
 func empty(){}
 func noReturn(){action();for{}}
 func assumed(p *int){if p!=nil{action()}}
 `)
	for _, test := range []struct {
		name       string
		any, every bool
	}{
		{"exact", true, true}, {"conditional", true, false}, {"empty", false, false}, {"noReturn", true, false}, {"assumed", true, true},
	} {
		for _, coverage := range []CompletionCoverage{CoverageEveryReturn, CoverageAnywhere} {
			t.Run(test.name+map[CompletionCoverage]string{CoverageEveryReturn: "Every", CoverageAnywhere: "Anywhere"}[coverage], func(t *testing.T) {
				function := pkg.Func(test.name)
				var nonNil ssa.Value
				if test.name == "assumed" {
					nonNil = function.Params[0]
				}
				calls := func(instruction ssa.Instruction) bool {
					common := ssaflow.InstructionCall(instruction)
					return common != nil && ssaflow.CallName(common) == "action"
				}
				want := test.every
				if coverage == CoverageAnywhere {
					want = test.any
				}
				if MethodCallCoverage(function, calls, coverage, nonNil) != want {
					t.Fatal("default contract changed")
				}
				for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
					budget := ssaflow.NewSearchBudget(limit)
					proof := ProveMethodCallCoverageWithin(function, calls, coverage, nonNil, budget)
					if budget.Exhausted() || limit == 0 {
						if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted {
							t.Fatalf("cut%d=%+v", limit, proof)
						}
						continue
					}
					if proof.State == ssaflow.EvidenceUnknown || proof.Proven() != want {
						t.Fatalf("complete%d=%+v want%v", limit, proof, want)
					}
					return
				}
				t.Fatal("coverage never completed")
			})
		}
	}
}

func TestMethodCoverageChildAndFresh(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "coveragechild", `package coveragechild;func action(){};func exact(){action()}`)
	calls := func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		return common != nil && ssaflow.CallName(common) == "action"
	}
	pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	cut := ProveMethodCallCoverageWithin(pkg.Func("exact"), calls, CoverageEveryReturn, nil, pool.Within(1))
	if cut.State != ssaflow.EvidenceUnknown || cut.Reason != ssaflow.EvidenceBudgetExhausted || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	fresh := ProveMethodCallCoverageWithin(pkg.Func("exact"), calls, CoverageEveryReturn, nil, pool.Within(ssaflow.SummaryBudget))
	if !fresh.Proven() {
		t.Fatalf("fresh=%+v", fresh)
	}
}
