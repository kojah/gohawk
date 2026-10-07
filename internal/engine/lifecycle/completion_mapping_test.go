package lifecycle

import (
	"bytes"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// A receiver captured by a closure is spilled to a cell. When the cell is
// written once, the lock's owner and a later argument are two loads of one
// value, and the lock is storage beneath that argument.
func TestSameValueStorageOwner(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerprobe", `package ownerprobe
import "sync"
type T struct{ mu sync.Mutex; n int }
type N struct{ child T }
func use(*T) {}
func nested(s *N) { s.child.mu.Lock(); use(&s.child) }
func once(s *T) {
	keep := func() { s.n++ }
	_ = keep
	s.mu.Lock()
	use(s)
}

func twice(s, other *T) {
	keep := func() { s.n++ }
	_ = keep
	s.mu.Lock()
	s = other
	use(s)
}
`)
	for name, want := range map[string]bool{"once": true, "twice": false, "nested": true} {
		var target, argument ssa.Value
		for _, block := range pkg.Func(name).Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				switch ssaflow.CallName(call.Common()) {
				case "Lock":
					target = ssaflow.CallReceiver(call.Common())
				case "use":
					argument = call.Common().Args[0]
				}
			}
		}
		if target == nil || argument == nil {
			t.Fatalf("%s: lock target or argument not found", name)
		}
		checkStorageOwnerAllowance(t, target, argument, want)
	}
}

// Deferred cleanup of embedded storage requires an unchanged owner cell and
// the exact field path. Pointee writes do not replace the owner pointer.
func TestDeferredCapturedStorageOwner(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerprobe", `package ownerprobe
import "sync"
type T struct { mu, other sync.Mutex; n int }
func exact(s *T) {
 s.mu.Lock()
 defer func() { s.n = 0; s.mu.Unlock() }()
}
func sibling(s *T) {
 s.mu.Lock()
 defer func() { s.other.Unlock() }()
}
func replacedAfter(s, other *T) {
 s.mu.Lock()
 defer func() { s.mu.Unlock() }()
 s = other
}
func replacedBefore(s, other *T) {
 s.mu.Lock()
 s = other
 defer func() { s.mu.Unlock() }()
}
func changedByCleanup(s, other *T) {
 s.mu.Lock()
 defer func() { s = other; s.mu.Unlock() }()
}
func expose(**T)
func opaqueCell(s *T) {
 s.mu.Lock()
 defer func() { s.mu.Unlock() }()
 expose(&s)
}
`)
	for name, want := range map[string]bool{
		"exact": true, "sibling": false, "replacedAfter": false,
		"replacedBefore": false, "changedByCleanup": false, "opaqueCell": false,
	} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var target ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
				if ssaflow.CallName(call.Common()) == "Lock" {
					target = ssaflow.CallReceiver(call.Common())
				}
			}
			deferred := ssaflow.InstructionsOf[*ssa.Defer](function)[0]
			proof := ProveCompletion(CompletionRequest{
				Instruction: deferred, Target: target, Methods: []string{"Unlock"},
				Budget: proofs.NewSearchBudget(proofs.QueryBudget),
			})
			if proof.Proven() != want {
				t.Errorf("deferred embedded cleanup = %+v, want proven %t", proof, want)
			}
		})
	}
}

func checkStorageOwnerAllowance(t *testing.T, target, argument ssa.Value, want bool) {
	t.Helper()
	if got := sameValueStorageOwner(target, argument, nil) != nil; got != want {
		t.Fatalf("default owner=%v, want %v", got, want)
	}
	for limit := 1; limit <= proofs.QueryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		owner := sameValueStorageOwner(target, argument, budget)
		if budget.Exhausted() {
			if owner != nil {
				t.Fatalf("cut %d publishes owner %v", limit, owner)
			}
			continue
		}
		if (owner != nil) != want {
			t.Fatalf("complete %d: owner=%v, want found %v", limit, owner, want)
		}
		return
	}
	t.Fatal("owner query never completed")
}

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
			full := proofs.NewSearchBudget(proofs.QueryBudget)
			baseline := newCompletionSearch("Close", CoverageEveryReturn, full).mappedLocals(callee, target, call)
			if full.Exhausted() || len(baseline) != 2 {
				t.Fatalf("baseline locals=%+v, exhausted=%v", baseline, full.Exhausted())
			}
			sawCut := false
			for allowance := 1; allowance < 100; allowance++ {
				budget := proofs.NewSearchBudget(allowance)
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
	function, closure := ssacall.DirectCallee(call.Common())
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
 func deep(p *node){take(p.`+strings.Repeat("child.", proofs.QueryBudget)+`child)}
 `)
	fn := pkg.Func("deep")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	search := newCompletionSearch("Close", CoverageEveryReturn, proofs.NewSearchBudget(100*proofs.QueryBudget))
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

// A field target requires the original aggregate at the receiver's read.
// Saved values and agreeing writes preserve that identity; derivation alone
// cannot credit replacement contents or ambiguous branch writes.
func TestCompletionSpillReplacement(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 type box struct {first *resource}
 func acquire()*resource{return new(resource)}
 func (b box)Close(){b.first.Close()}
 func replacement(b,c box){b=c;b.first.Close()}
 func earlier(b,c box){p:=b.first;b=c;p.Close()}
 func wrapped(b,c box){p:=b.first;b=c;var closer interface{Close()}=p;closer.Close()}
 func restored(b,c box){original:=b;b=c;p:=b.first;b=original;p.Close()}
 func ambiguous(b,c box,flag bool){if flag{b=c};b.first.Close()}
 func agreeing(b,c box){original:=b;if c.first!=nil{b=original};b.first.Close()}
 func whole(b,c box){b.Close()}
 func dynamic(files [2]*resource){for _,r:=range files{r.Close()}}
 func runReplacement(q *resource){p:=acquire();replacement(box{p},box{q})}
 func runEarlier(q *resource){p:=acquire();earlier(box{p},box{q})}
 func runWrapped(q *resource){p:=acquire();wrapped(box{p},box{q})}
 func runRestored(q *resource){p:=acquire();restored(box{p},box{q})}
 func runAmbiguous(q *resource,flag bool){p:=acquire();ambiguous(box{p},box{q},flag)}
 func runAgreeing(q *resource){p:=acquire();agreeing(box{p},box{q})}
 func runWhole(q *resource){p:=acquire();whole(box{p},box{q})}
 func runDynamic(q *resource){p:=acquire();dynamic([2]*resource{p,q})}
 `)
	for _, test := range []struct {
		name      string
		completes bool
	}{
		{"runReplacement", false},
		{"runEarlier", true},
		{"runWrapped", true},
		{"runRestored", false},
		{"runAmbiguous", false},
		{"runAgreeing", true},
		{"runWhole", true},
		{"runDynamic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](fn)
			if len(calls) != 2 || ssaflow.CallName(calls[0].Common()) != "acquire" {
				t.Fatal("expected acquisition followed by cleanup helper")
			}
			var ir bytes.Buffer
			if _, err := calls[1].Common().StaticCallee().WriteTo(&ir); err != nil {
				t.Fatal(err)
			}
			t.Log(ir.String())
			request := CompletionRequest{Instruction: calls[1], Target: calls[0], Methods: []string{"Close"}}
			request.Budget = proofs.NewSearchBudget(10 * proofs.SummaryBudget)
			proof := ProveCompletion(request)
			if proof.Proven() != test.completes {
				t.Fatalf("completion=%+v, want %v", proof, test.completes)
			}
			if test.name == "runDynamic" && proof.Reason != proofs.EvidenceCompletionInCycle {
				t.Fatalf("dynamic cleanup lost loop uncertainty: %+v", proof)
			}
			if test.completes {
				assertCompletionSpillCutoff(t, request)
			}
		})
	}
}

func assertCompletionSpillCutoff(t *testing.T, request CompletionRequest) {
	t.Helper()
	sawCut := false
	for limit := 1; limit <= 10*proofs.SummaryBudget; limit += 10 {
		request.Budget = proofs.NewSearchBudget(limit)
		proof := ProveCompletion(request)
		if request.Budget.Exhausted() {
			sawCut = true
			if proof.Proven() || proof.PathKnown {
				t.Fatalf("cutoff %d published completion %+v", limit, proof)
			}
			continue
		}
		if !sawCut || !proof.Proven() {
			t.Fatalf("fresh allowance %d failed to recover: %+v", limit, proof)
		}
		return
	}
	t.Fatal("completion never recovered within allowance")
}
