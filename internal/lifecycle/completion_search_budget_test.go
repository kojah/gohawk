package lifecycle

import (
	"testing"

	"golang.org/x/tools/go/ssa"

	"github.com/kojah/gohawk/internal/ssaflow"
)

const completionCoverageBudgetFixture = `package ssaflowtest
 type resource struct{}
 func(*resource)Close(){}
 func finish(p *resource){p.Close()}
 func maybe(p *resource,yes bool){if yes{p.Close()}}
 func nested(p *resource,yes bool){maybe(p,yes)}
 func typed(x interface{}){if p,ok:=x.(*resource);ok{p.Close()}}
 func readOnly(p *resource){}
 func caseTrue(p *resource,yes bool)bool{if yes{p.Close();return true};return false}
 func argumentCase(p *resource,yes bool){if yes{p.Close()}}
 func runExact(p *resource){finish(p)}
 func runTrue(p *resource){maybe(p,true)}
 func runFalse(p *resource){maybe(p,false)}
 func runNested(p *resource){nested(p,true)}
 func runType(p *resource){typed(p)}
 func runMissing(p *resource){readOnly(p)}
`

func TestCompletionCoverageSearchAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"runExact", true}, {"runTrue", true}, {"runFalse", false}, {"runNested", true}, {"runType", true}, {"runMissing", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			request := CompletionRequest{Instruction: findLaunch(t, fn), Target: fn.Params[0], Methods: []string{"Close"}}
			baseline := ProveCompletion(request)
			if baseline.Proven() != test.want {
				t.Fatalf("default=%+v", baseline)
			}
			for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
				request.Budget = ssaflow.NewSearchBudget(limit)
				got := ProveCompletion(request)
				if request.Budget.Exhausted() {
					if got.State != ssaflow.EvidenceUnknown || got.Reason != ssaflow.EvidenceBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				if got.State == ssaflow.EvidenceUnknown || got.Proven() != test.want {
					t.Fatalf("complete%d=%+v want%v", limit, got, test.want)
				}
				return
			}
			t.Fatal("query never completed")
		})
	}
}

func TestCompletionCoverageMemoChildAndFresh(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	fn := pkg.Func("runNested")
	call := findLaunch(t, fn)
	pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	search := newCompletionSearch("Close", CoverageEveryReturn, pool.Within(1))
	if got := search.completes(call, fn.Params[0]); got.proven || !search.budget.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", got, pool.Exhausted())
	}
	search.budget = pool.Within(ssaflow.SummaryBudget)
	if got := search.completes(call, fn.Params[0]); !got.proven {
		t.Fatalf("fresh memo query=%+v", got)
	}
}

func TestCompletionCaseCoverageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, test := range []struct {
		name      string
		condition ssaflow.CallCondition
	}{
		{"caseTrue", ssaflow.CallCondition{Outcome: ssaflow.OutcomeTrue}},
		{"argumentCase", ssaflow.CallCondition{Arguments: ssaflow.ArgumentConstants{Bound: 2, Values: 2}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			request := CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}}
			for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
				request.Budget = ssaflow.NewSearchBudget(limit)
				proof := ProveCompletionForCase(fn, test.condition, request)
				if request.Budget.Exhausted() {
					if proof.Proven() || proof.Reason != ssaflow.EvidenceBudgetExhausted {
						t.Fatalf("cut case%d=%+v", limit, proof)
					}
					continue
				}
				if !proof.Proven() {
					t.Fatalf("complete case%d=%+v", limit, proof)
				}
				return
			}
			t.Fatal("case never completed")
		})
	}
}

func TestCompletionUnavailableSummaryCut(t *testing.T) {
	pkg := buildTestSSA(t, completionOutcomeFixture)
	fn := pkg.Func("callOpaque")
	pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	request := CompletionRequest{Instruction: findLaunch(t, fn), Target: fn.Params[0], Methods: []string{"Close"}, Budget: pool.Within(0)}
	request.Summarized = func(ssa.Instruction, ssa.Value, string, bool, ssaflow.CallCondition) bool {
		return request.Budget.Spend()
	}
	cut := ProveCompletion(request)
	if cut.State != ssaflow.EvidenceUnknown || cut.Reason != ssaflow.EvidenceBudgetExhausted || pool.Exhausted() {
		t.Fatalf("unavailable cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	request.Budget = pool.Within(ssaflow.SummaryBudget)
	if fresh := ProveCompletion(request); !fresh.Proven() {
		t.Fatalf("fresh summary=%+v", fresh)
	}
}

func TestCompletionAssumedCoverageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, yes := range []bool{false, true} {
		fn := pkg.Func("maybe")
		outcome := ssaflow.OutcomeFalse
		if yes {
			outcome = ssaflow.OutcomeTrue
		}
		assumptions := ssaflow.EntryAssumptions{NonNil: fn.Params[0], Constants: ssaflow.FixedValues{fn.Params[1]: outcome}}
		calls := func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			return common != nil && ssaflow.CallName(common) == "Close"
		}
		completed := false
		for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
			budget := ssaflow.NewSearchBudget(limit)
			proof := proveMethodCallCoverageAssumingWithin(fn, calls, CoverageEveryReturn, assumptions, budget)
			if budget.Exhausted() || limit == 0 {
				if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted {
					t.Fatalf("assumed cut%d=%+v", limit, proof)
				}
				continue
			}
			if proof.State == ssaflow.EvidenceUnknown || proof.Proven() != yes {
				t.Fatalf("assumed complete%d=%+v want%v", limit, proof, yes)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("assumed query never completed")
		}
	}
}
