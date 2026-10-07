package lifecycle

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const callbackBindingsFixture = `
package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func (*resource) Wait() {}
func invoke(fn func(*resource), r *resource) { fn(r) }
func forward(fn func(*resource), r *resource) { invoke(fn, r) }
func conditional(fn func(*resource), r *resource, yes bool) { if yes { fn(r) } }
func twice(first, second func(*resource), r *resource) { invoke(first, r); invoke(second, r) }
func closeResource(r *resource) { r.Close() }
func noop(r *resource) {}
func direct(r *resource) { invoke(closeResource, r) }
func literal(r *resource) { invoke(func(v *resource) { v.Close() }, r) }
func forwarded(r *resource) { forward(closeResource, r) }
func waiting(r *resource) { invoke(func(v *resource) { v.Wait() }, r) }
func skipped(r *resource, yes bool) { conditional(closeResource, r, yes) }
func wrong(r, other *resource) { invoke(closeResource, other) }
func empty(r *resource) { invoke(noop, r) }
func mixed(r *resource, yes bool) {
    fn := closeResource
    if yes { fn = noop }
    invoke(fn, r)
}
func opaque(r *resource, fn func(*resource)) { invoke(fn, r) }
func separate(r *resource) { twice(noop, closeResource, r) }
func branchInvoke(a, b func(*resource), r *resource, yes bool) {
    if yes { invoke(a, r) } else { invoke(b, r) }
}
func branchMixed(r *resource, yes bool) { branchInvoke(closeResource, noop, r, yes) }
func branchComplete(r *resource, yes bool) { branchInvoke(closeResource, closeResource, r, yes) }
func recurse(fn func(*resource), r *resource) { recurse(fn, r) }
func recursive(r *resource) { recurse(closeResource, r) }
func retained(r *resource) { hold(closeResource, r) }
var saved func(*resource)
func hold(fn func(*resource), r *resource) { saved = fn }
func captured(r, other *resource) { invoke(func(v *resource) { _ = other; v.Close() }, r) }
func wrongCapture(r, other *resource) { invoke(func(v *resource) { other.Close() }, r) }
func replace(fn func(*resource), r *resource) { fn = noop; fn(r) }
func reassigned(r *resource) { replace(closeResource, r) }
func capturedInvoke(fn func(*resource), r *resource) { func() { fn(r) }() }
func nested(r *resource) { capturedInvoke(closeResource, r) }
func capturedMutate(fn func(*resource), r *resource) {
    change := func() { fn = noop }
    change()
    func() { fn(r) }()
}
func changedCapture(r *resource) { capturedMutate(closeResource, r) }
type callbacks struct { fn func(*resource) }
func fieldInvoke(c *callbacks, r *resource) { c.fn(r) }
func field(r *resource) { fieldInvoke(&callbacks{fn: closeResource}, r) }
func fieldEmpty(r *resource) { fieldInvoke(&callbacks{}, r) }
func fieldWrong(r *resource) { fieldInvoke(&callbacks{fn: noop}, r) }
func elementInvoke(c []func(*resource), r *resource) { c[0](r) }
func element(r *resource) { elementInvoke([]func(*resource){closeResource}, r) }
func elementWrong(r *resource) { elementInvoke([]func(*resource){noop}, r) }
func dynamicInvoke(c []func(*resource), r *resource, i int) { c[i](r) }
func dynamic(r *resource, i int) { dynamicInvoke([]func(*resource){closeResource, closeResource}, r, i) }
func dynamicMixed(r *resource, i int) { dynamicInvoke([]func(*resource){closeResource, noop}, r, i) }
func dynamicHole(r *resource, i int) { dynamicInvoke([]func(*resource){closeResource, nil}, r, i) }
func fieldMutator(c *callbacks, r *resource) { c.fn = noop; c.fn(r) }
func mutatedField(r *resource) { fieldMutator(&callbacks{fn:closeResource}, r) }
func elementMutator(c []func(*resource), r *resource) { c[0] = noop; c[0](r) }
func mutatedElement(r *resource) { elementMutator([]func(*resource){closeResource}, r) }
type holder struct { item *resource }
func wrappedInvoke(fn func(*holder), h *holder) { fn(h) }
func wrapped(r *resource) { wrappedInvoke(func(h *holder) { h.item.Close() }, &holder{r}) }
func wrappedWrong(r, other *resource) { wrappedInvoke(func(h *holder) { h.item.Close() }, &holder{other}) }
func escapeCallbacks(c *callbacks)
func escapedField(r *resource) { c := &callbacks{closeResource}; escapeCallbacks(c); fieldInvoke(c, r) }
func escapeSlice(c []func(*resource))
func escapedElement(r *resource) { c := []func(*resource){closeResource}; escapeSlice(c); elementInvoke(c, r) }
func sliced(r *resource) { c := []func(*resource){closeResource, noop}; elementInvoke(c[1:], r) }
`

func TestDirectCallbackBindings(t *testing.T) {
	pkg := buildTestSSA(t, callbackBindingsFixture)
	for _, test := range []struct {
		name   string
		method string
		proven bool
	}{
		{"direct", "Close", true},
		{"literal", "Close", true},
		{"forwarded", "Close", true},
		{"waiting", "Wait", true},
		{"skipped", "Close", false},
		{"wrong", "Close", false},
		{"empty", "Close", false},
		{"mixed", "Close", false},
		{"opaque", "Close", false},
		{"separate", "Close", true},
		{"branchMixed", "Close", false},
		{"branchComplete", "Close", true},
		{"recursive", "Close", false},
		{"retained", "Close", false},
		{"captured", "Close", true},
		{"wrongCapture", "Close", false},
		{"reassigned", "Close", false},
		{"nested", "Close", true},
		{"changedCapture", "Close", false},
		{"field", "Close", true},
		{"fieldEmpty", "Close", false},
		{"fieldWrong", "Close", false},
		{"element", "Close", true},
		{"elementWrong", "Close", false},
		{"dynamic", "Close", true},
		{"dynamicMixed", "Close", false},
		{"dynamicHole", "Close", false},
		{"mutatedField", "Close", false},
		{"mutatedElement", "Close", false},
		{"wrapped", "Close", true},
		{"wrappedWrong", "Close", false},
		{"escapedField", "Close", false},
		{"escapedElement", "Close", false},
		{"sliced", "Close", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](fn)
			proof := ProveCompletion(CompletionRequest{
				Instruction: calls[len(calls)-1], Target: fn.Params[0],
				Methods: []string{test.method}, Budget: proofs.NewSearchBudget(1000),
			})
			if proof.Proven() != test.proven {
				t.Fatalf("proof = %+v, want proven %v", proof, test.proven)
			}
		})
	}
}

func BenchmarkCallbackBindings(b *testing.B) {
	pkg := ssaflowtest.BuildPackage(b, "ssaflowtest", callbackBindingsFixture)
	for _, name := range []string{"direct", "forwarded", "nested", "field", "dynamic"} {
		b.Run(name, func(b *testing.B) {
			fn := pkg.Func(name)
			var call ssa.Instruction
			for _, instruction := range ssaflow.InstructionsOf[*ssa.Call](fn) {
				call = instruction
				break
			}
			b.ReportAllocs()
			for b.Loop() {
				proof := ProveCompletion(CompletionRequest{
					Instruction: call, Target: fn.Params[0],
					Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1000),
				})
				if !proof.Proven() {
					b.Fatalf("proof = %+v", proof)
				}
			}
		})
	}
}

func TestUnresolvedCallbackIsUnknown(t *testing.T) {
	pkg := buildTestSSA(t, callbackBindingsFixture)
	fn := pkg.Func("opaque")
	proof := ProveCompletion(CompletionRequest{
		Instruction: findLaunch(t, fn), Target: fn.Params[0],
		Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1000),
	})
	if proof.State != proofs.EvidenceUnknown {
		t.Fatalf("unresolved callback = %+v", proof)
	}
}

func TestCallbackBindingsBudget(t *testing.T) {
	pkg := buildTestSSA(t, callbackBindingsFixture)
	fn := pkg.Func("forwarded")
	proof := ProveCompletion(CompletionRequest{
		Instruction: findLaunch(t, fn), Target: fn.Params[0],
		Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1),
	})
	if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("exhausted callback search = %+v", proof)
	}
}

func TestCompletionFixedBindingCutoff(t *testing.T) {
	source := completionCoverageBudgetFixture + `
 func many(p *resource,flag *int){p.Close();` + strings.Repeat("println(flag);", proofs.QueryBudget+1) + `if flag==nil{println("nil")}}
 func runMany(p *resource){many(p,nil)}
 `
	fn := buildTestSSA(t, source).Func("runMany")
	call := findLaunch(t, fn)
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	request := CompletionRequest{Instruction: call, Target: fn.Params[0], Methods: []string{"Close"}, Coverage: CoverageAnywhere, Budget: child}
	proof := ProveCompletion(request)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("binding child=%+v, parent exhausted=%v", proof, pool.Exhausted())
	}
	request.Budget = pool.Within(2 * proofs.SummaryBudget)
	if proof := ProveCompletion(request); !proof.Proven() {
		t.Fatalf("fresh completion=%+v", proof)
	}
	search := newCompletionSearch("Close", CoverageAnywhere, pool.Within(proofs.QueryBudget))
	if got := search.completes(call, fn.Params[0]); got.proven || !search.budget.Exhausted() {
		t.Fatalf("memo child=%+v", got)
	}
	search.budget = pool.Within(2 * proofs.SummaryBudget)
	if got := search.completes(call, fn.Params[0]); !got.proven {
		t.Fatalf("fresh memo=%+v", got)
	}
}

func TestResultGuardNamedCellCensusReuse(t *testing.T) {
	fn := buildTestSSA(t, `package ssaflowtest
 func repeated()(err error){
  defer func(){println(err)}()
  defer func(){println(err)}()
  defer func(){println(err)}()
  return nil
 }
 `).Func("repeated")
	closures := ssaflow.InstructionsOf[*ssa.MakeClosure](fn)
	if len(closures) != 3 {
		t.Fatalf("closures=%d", len(closures))
	}
	pool := proofs.NewSearchBudget(10000)
	var named ssaflow.NamedResultCellsProof
	first := capturedResultCells(fn, closures[0], pool.Within(proofs.SummaryBudget), &named)
	if !named.Proven() || len(first) != 1 || len(named.Cells) != 1 {
		t.Fatalf("first=%v census=%+v", first, named)
	}
	for _, closure := range closures[1:] {
		budget := pool.Within(len(closure.Bindings))
		cells := capturedResultCells(fn, closure, budget, &named)
		if budget.Exhausted() || pool.Exhausted() || !reflect.DeepEqual(cells, first) {
			t.Fatalf("warm=%v first=%v exhausted=%v", cells, first, budget.Exhausted())
		}
	}
	var cold ssaflow.NamedResultCellsProof
	cut := pool.Within(3)
	if cells := capturedResultCells(fn, closures[0], cut, &cold); len(cells) != 0 || cold.Proven() || cold.Cells != nil || !cut.Exhausted() {
		t.Fatalf("cut=%v census=%+v", cells, cold)
	}
	fresh := pool.Within(proofs.SummaryBudget)
	cells := capturedResultCells(fn, closures[0], fresh, &cold)
	if fresh.Exhausted() || pool.Exhausted() || !reflect.DeepEqual(cells, first) || !reflect.DeepEqual(cold, named) {
		t.Fatalf("fresh=%v census=%+v", cells, cold)
	}
}

func TestCompletionForwardedFields(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 type box struct{first,second *resource}
 func acquire()*resource{return new(resource)}
 func closeOne(p *resource){p.Close()}
 func first(b *box){closeOne(b.first)}
 func earlier(b,c box){p:=b.first;b=c;closeOne(p)}
 func replacement(b,c box){b=c;closeOne(b.first)}
 func ambiguous(b,c box,flag bool){if flag{b=c};closeOne(b.first)}
 func runFirst(q *resource){p:=acquire();first(&box{p,q})}
 func runSibling(q *resource){p:=acquire();first(&box{q,p})}
 func runEarlier(q *resource){p:=acquire();earlier(box{p,q},box{q,q})}
 func runEarlierSibling(q *resource){p:=acquire();earlier(box{q,p},box{q,q})}
 func runReplacement(q *resource){p:=acquire();replacement(box{p,q},box{q,q})}
 func runAmbiguous(q *resource,flag bool){p:=acquire();ambiguous(box{p,q},box{q,q},flag)}
 `)
	for _, test := range []struct {
		name      string
		completes bool
	}{
		{"runFirst", true},
		{"runSibling", false},
		{"runEarlier", true},
		{"runEarlierSibling", false},
		{"runReplacement", false},
		{"runAmbiguous", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(test.name))
			if len(calls) != 2 || ssaflow.CallName(calls[0].Common()) != "acquire" {
				t.Fatal("expected acquisition followed by cleanup helper")
			}
			var ir bytes.Buffer
			if _, err := calls[1].Common().StaticCallee().WriteTo(&ir); err != nil {
				t.Fatal(err)
			}
			t.Log(ir.String())
			request := CompletionRequest{
				Instruction: calls[1], Target: calls[0], Methods: []string{"Close"},
				Budget: proofs.NewSearchBudget(10 * proofs.SummaryBudget),
			}
			proof := ProveCompletion(request)
			if proof.Proven() != test.completes {
				t.Fatalf("completion=%+v, want %v", proof, test.completes)
			}
			if test.completes {
				if !proof.PathKnown || proof.Path != "" {
					t.Fatalf("exact field target lost path: %+v", proof)
				}
				assertCompletionSpillCutoff(t, request)
			}
		})
	}
}

func TestDeferredCellMappingSharesRequestAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
func deferredCleared(p *resource, pick bool){held:=p;defer func(){if held!=nil{held.Close()}}();if pick{p.Close();held=nil}}
`)
	function := pkg.Func("deferredCleared")
	deferred := ssaflow.InstructionsOf[*ssa.Defer](function)[0]
	body, closure := ssacall.DirectCallee(deferred.Common())
	if closure == nil || len(closure.Bindings) != 1 {
		t.Fatal("expected captured deferred cell")
	}
	cell, ok := closure.Bindings[0].(*ssa.Alloc)
	if !ok {
		t.Fatalf("capture is %T, want actual cell", closure.Bindings[0])
	}
	pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
	search := newCompletionSearch("Close", CoverageEveryReturn, pool.Within(1))
	if _, mapped := search.deferredCellLocal(body.FreeVars[0], cell, function.Params[0], deferred); mapped || !search.budget.Exhausted() {
		t.Fatal("graph relation bypassed the exhausted mapping allowance")
	}
	if pool.Exhausted() {
		t.Fatal("independent child cutoff exhausted the pool")
	}
	search.budget = pool.Within(proofs.QueryBudget)
	local, mapped := search.deferredCellLocal(body.FreeVars[0], cell, function.Params[0], deferred)
	if !mapped || local.kind != localExact || search.budget.Exhausted() {
		t.Fatalf("fresh exact cell mapping did not recover: %+v, mapped=%v", local, mapped)
	}
}

// Exact cleanup crosses only the selected identity wrappers. A phi or load
// cannot become the parameter just because its possible origins contain it.
func TestExactCleanupReceiverAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 type same resource
 func observe(interface{}){}
 func wrapped(p *resource){observe(p)}
 func converted(p *resource){observe((*same)(p))}
 func merged(p,q *resource,flag bool){v:=p;if flag{v=q};observe(v)}
 func loaded(p **resource){observe(*p)}
 `)
	for _, test := range []struct {
		name string
		want bool
	}{{"wrapped", true}, {"converted", true}, {"merged", false}, {"loaded", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var receiver ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
				if ssaflow.CallName(call.Common()) == "observe" {
					receiver = call.Common().Args[0]
				}
			}
			if receiver == nil {
				t.Fatal("receiver not found")
			}
			if test.want {
				cut := proofs.NewSearchBudget(1)
				if exactCleanupReceiver(receiver, fn.Params[0], cut) || !cut.Exhausted() {
					t.Fatal("wrapper traversal bypassed allowance")
				}
			}
			for limit := 1; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := exactCleanupReceiver(receiver, fn.Params[0], budget)
				if budget.Exhausted() {
					if got {
						t.Fatal("cut proves exact receiver")
					}
					continue
				}
				if got != test.want {
					t.Fatalf("complete %d match=%v", limit, got)
				}
				return
			}
			t.Fatal("receiver query never completed")
		})
	}
}
