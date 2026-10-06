package lifecycle

import (
	"testing"

	"golang.org/x/tools/go/ssa"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
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
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				request.Budget = proofs.NewSearchBudget(limit)
				got := ProveCompletion(request)
				if request.Budget.Exhausted() {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				if got.State == proofs.EvidenceUnknown || got.Proven() != test.want {
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
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	search := newCompletionSearch("Close", CoverageEveryReturn, pool.Within(1))
	if got := search.completes(call, fn.Params[0]); got.proven || !search.budget.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", got, pool.Exhausted())
	}
	search.budget = pool.Within(proofs.SummaryBudget)
	if got := search.completes(call, fn.Params[0]); !got.proven {
		t.Fatalf("fresh memo query=%+v", got)
	}
}

func TestCompletionCaseCoverageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, test := range []struct {
		name      string
		condition ssacall.CallCondition
	}{
		{"caseTrue", ssacall.CallCondition{Outcome: ssacall.OutcomeTrue}},
		{"argumentCase", ssacall.CallCondition{Arguments: ssacall.ArgumentConstants{Bound: 2, Values: 2}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			request := CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				request.Budget = proofs.NewSearchBudget(limit)
				proof := ProveCompletionForCase(fn, test.condition, request)
				if request.Budget.Exhausted() {
					if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
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
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	request := CompletionRequest{Instruction: findLaunch(t, fn), Target: fn.Params[0], Methods: []string{"Close"}, Budget: pool.Within(0)}
	request.Summarized = func(ssa.Instruction, ssa.Value, string, bool, ssacall.CallCondition) bool {
		return request.Budget.Spend()
	}
	cut := ProveCompletion(request)
	if cut.State != proofs.EvidenceUnknown || cut.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
		t.Fatalf("unavailable cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	request.Budget = pool.Within(proofs.SummaryBudget)
	if fresh := ProveCompletion(request); !fresh.Proven() {
		t.Fatalf("fresh summary=%+v", fresh)
	}
}

func TestCompletionAssumedCoverageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, yes := range []bool{false, true} {
		fn := pkg.Func("maybe")
		outcome := ssacall.OutcomeFalse
		if yes {
			outcome = ssacall.OutcomeTrue
		}
		assumptions := ssapath.EntryAssumptions{NonNil: fn.Params[0], Constants: ssacall.FixedValues{fn.Params[1]: outcome}}
		calls := func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			return common != nil && ssaflow.CallName(common) == "Close"
		}
		completed := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			budget := proofs.NewSearchBudget(limit)
			proof := proveMethodCallCoverageAssumingWithin(fn, calls, CoverageEveryReturn, assumptions, budget)
			if budget.Exhausted() || limit == 0 {
				if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
					t.Fatalf("assumed cut%d=%+v", limit, proof)
				}
				continue
			}
			if proof.State == proofs.EvidenceUnknown || proof.Proven() != yes {
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
