package lifecycle

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A local map is useful only after all supplied bindings have been examined.
// A completing prefix must not survive a later mapping or metadata cutoff.
func TestCompletionMappingCutoffDiscardsPrefix(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 func settlePair(a,b *resource){a.Close();b.Close()}
 func mapArguments(p *resource){settlePair(p,p)}
 func mapCaptures(p *resource){q:=p;f:=func(){p.Close();q.Close()};f()}
 `)
	for _, name := range []string{"mapArguments", "mapCaptures"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			callee, target, call := mappingBudgetCase(t, fn)
			full := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
			baseline := newCompletionSearch("Close", CoverageEveryReturn, full).mappedLocals(callee, target, call)
			if full.Exhausted() || len(baseline) != 2 {
				t.Fatalf("baseline locals=%+v, exhausted=%v", baseline, full.Exhausted())
			}
			sawCut := false
			for allowance := 1; allowance < 100; allowance++ {
				budget := ssaflow.NewSearchBudget(allowance)
				locals := newCompletionSearch("Close", CoverageEveryReturn, budget).mappedLocals(callee, target, call)
				if budget.Exhausted() {
					sawCut = true
					if len(locals) != 0 {
						t.Fatalf("allowance %d published %d partial locals", allowance, len(locals))
					}
					continue
				}
				if len(locals) != len(baseline) {
					t.Fatalf("completed allowance %d mapped %d locals", allowance, len(locals))
				}
				if !sawCut {
					t.Fatal("no cutoff exercised")
				}
				return
			}
			t.Fatal("mapping did not complete within test allowance")
		})
	}
}

func mappingBudgetCase(t *testing.T, fn *ssa.Function) (completionCallee, ssa.Value, *ssa.Call) {
	t.Helper()
	var call *ssa.Call
	for _, candidate := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(candidate.Common()) == "settlePair" {
			call = candidate
			break
		}
		if _, ok := candidate.Common().Value.(*ssa.MakeClosure); ok {
			call = candidate
			break
		}
	}
	if call == nil {
		t.Fatal("mapping call not found")
	}
	function, closure := ssaflow.DirectCallee(call.Common())
	callee := completionCallee{common: call.Common(), function: function, closure: closure, launch: launchCalled}
	var target ssa.Value = fn.Params[0]
	if closure != nil {
		// Capturing p spills it; use the actual value read before q is bound.
		target = ssaflow.CapturedBindingValue(closure.Bindings[0])
	}
	return callee, target, call
}

// A strict projection's own child may stop while the request remains usable.
// That shortened answer must invalidate the enclosing completion memo too.
func TestMappingChildCutoffInvalidatesMemo(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type node struct {child *node}
 func(*node)Close(){}
 func take(*node){}
 func deep(p *node){take(p.`+strings.Repeat("child.", ssaflow.QueryBudget)+`child)}
 `)
	fn := pkg.Func("deep")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	search := newCompletionSearch("Close", CoverageEveryReturn, ssaflow.NewSearchBudget(100*ssaflow.QueryBudget))
	key := completionKey{instruction: call, target: fn.Params[0]}
	attempts := 0
	for range 2 {
		search.memo.Answer(key, func() completionAnswer {
			attempts++
			_, ok := search.argumentLocal(call.Common().StaticCallee().Params[0], call.Common().Args[0], fn.Params[0], call)
			if ok || search.budget.Exhausted() || !*search.incomplete {
				t.Fatalf("child mapping=%v, exhausted=%v, incomplete=%v", ok, search.budget.Exhausted(), *search.incomplete)
			}
			return completionAnswer{available: true}
		})
	}
	if attempts != 2 {
		t.Fatal("memo retained a child-cut mapping answer")
	}
}
