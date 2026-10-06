package lifecycle

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredStoreQueriesShareAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 func acquire()*resource{return new(resource)}
 func other()*resource{return new(resource)}
 func observe(**resource){}
 func conditional(flag bool){var p *resource;if flag{p=acquire()}else{p=other()};defer func(){p.Close()}()}
 func after(flag bool){var p *resource;if flag{p=acquire()}else{p=other()};defer func(){p.Close()}();p=other()}
 func between(flag bool){var p *resource;if flag{p=acquire()}else{p=other()};p=other();defer func(){p.Close()}()}
 func fresh(flag bool){for flag{p:=acquire();defer func(){p.Close()}()}}
 func padded(flag bool){var p *resource;if flag{p=acquire()}else{p=other()};`+
		strings.Repeat("observe(&p);", 80)+`defer func(){p.Close()}()}
 func empty(){var p *resource;`+strings.Repeat("observe(&p);", 80)+`defer func(){if p!=nil{p.Close()}}()}
 `)
	for _, test := range []struct {
		name   string
		proven bool
	}{
		{"conditional", true}, {"after", false}, {"between", false}, {"fresh", true}, {"padded", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			cell := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
			deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
			var target ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
				if ssaflow.CallName(call.Common()) == "acquire" {
					target = call
				}
			}
			if target == nil {
				t.Fatal("missing acquisition")
			}
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			full := targetStoredOnPath(cell, target, deferred, proofs.NewSearchBudget(10*proofs.QueryBudget))
			if full.Proven() != test.proven {
				t.Fatalf("target store=%+v, want %v", full, test.proven)
			}
			cut := proofs.NewSearchBudget(1)
			proof := targetStoredOnPath(cell, target, deferred, cut)
			if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted || !cut.Exhausted() {
				t.Fatalf("cutoff published target store: %+v", proof)
			}
			if test.name == "padded" {
				cut = proofs.NewSearchBudget(20)
				proof = targetStoredOnPath(cell, target, deferred, cut)
				if proof.Proven() || !cut.Exhausted() {
					t.Fatalf("prefix bypassed full census: %+v", proof)
				}
			}
		})
	}
	fn := pkg.Func("empty")
	cell := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
	cut := proofs.NewSearchBudget(1)
	if valueHasDirectStore(cell, cut) || !cut.Exhausted() {
		t.Fatal("direct-store census bypassed allowance")
	}
	if valueHasDirectStore(cell, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("zero-initialized cell has a direct store")
	}
}

func TestDeferredStableBindingChildCutoffInvalidatesMemo(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 type owner struct{first *resource}
 func(o *owner)Close(){o.first.Close()}
 type box struct{held owner}
 func observe(**resource){}
 func shallow(p,q *resource){b:=new(box);b.held=owner{q};f:=b.held.Close;defer f()}
 func deep(p,q *resource){b:=new(box);b.held=owner{q};f:=b.held.Close;defer f();`+
		strings.Repeat("observe(&b.held.first);", 300)+`}
 `)
	shallow := pkg.Func("shallow")
	shallowDefer := ssaflow.InstructionsOf[*ssa.Defer](shallow)[0]
	_, shallowClosure := ssacall.DirectCallee(shallowDefer.Common())
	if shallowClosure == nil || len(shallowClosure.Bindings) != 1 {
		t.Fatal("expected shallow field-address capture")
	}
	fresh := newCompletionSearch("Close", CoverageEveryReturn, proofs.NewSearchBudget(10*proofs.QueryBudget))
	proof := fresh.deferredBindingValue(shallowClosure.Bindings[0], shallow.Params[0], shallowDefer)
	if !proof.Proven() {
		t.Fatalf("fresh stable binding did not recover: %+v", proof)
	}
	fn := pkg.Func("deep")
	var dump strings.Builder
	if _, err := fn.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
	body, closure := ssacall.DirectCallee(deferred.Common())
	if closure == nil || len(closure.Bindings) != 1 {
		t.Fatal("expected field-address capture")
	}
	if _, ok := closure.Bindings[0].(*ssa.FieldAddr); !ok {
		t.Fatal("capture must be a field address")
	}
	callee := completionCallee{function: body, closure: closure, common: deferred.Common(), launch: launchDeferred}
	search := newCompletionSearch("Close", CoverageEveryReturn, proofs.NewSearchBudget(100*proofs.QueryBudget))
	key := completionKey{instruction: deferred, target: fn.Params[0]}
	attempts := 0
	for range 2 {
		search.memo.Answer(key, func() completionAnswer {
			attempts++
			_, ok := search.capturedLocal(callee, body.FreeVars[0], closure.Bindings[0], fn.Params[0], deferred)
			if ok || search.budget.Exhausted() || !*search.incomplete {
				t.Fatalf("child mapping=%v, request cut=%v, incomplete=%v", ok, search.budget.Exhausted(), *search.incomplete)
			}
			return completionAnswer{available: true}
		})
	}
	if attempts != 2 {
		t.Fatal("memo retained an independently cut stable binding")
	}
}
