package heapmodel

import (
	"go/token"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStorageOrderingAndDominanceCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "order", `package order
 func observe(a,b *int) {}
 func read(p **int) { _ = *p }
 func probe(a *int) { x:=a; read(&x); observe(x,a) }
`)
	function := pkg.Func("probe")
	call := heapObservation(t, function)
	load := call.Common().Args[0].(*ssa.UnOp)
	cell := load.X.(*ssa.Alloc)
	stores := ssaflow.InstructionsOf[*ssa.Store](function)
	if len(stores) != 1 {
		t.Fatal("expected a sole actual SSA initializer")
	}
	storage := NewStorage(proofs.NewSearchBudget(1))
	if proof := storage.reachingContent(storageLocation{root: cell}, load, stores); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("unavailable initializer dominance must stay unknown: %+v", proof)
	}
	fresh := NewStorage(proofs.NewSearchBudget(proofs.QueryBudget))
	if proof := fresh.reachingContent(storageLocation{root: cell}, load, stores); !proof.Proven() || proof.Value != call.Common().Args[1] {
		t.Fatal("fresh initializer dominance lost exact contents")
	}
	storage = NewStorage(proofs.NewSearchBudget(1))
	var collected []*ssa.Store
	_, ok := storage.collect(cell, load, &collected, false)
	if ok || !storage.Budget().Exhausted() || len(collected) != 0 {
		t.Fatal("a referrer must not be admitted after its order query cuts off")
	}
}

func TestStoreFollowingKeepsFreshAllocationBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storeorder", `package storeorder
 func observe(**int){}
 func after(p,q *int){x:=p;observe(&x);x=q}
 func before(p,q *int){x:=p;x=q;observe(&x)}
 func branch(p,q *int,flag bool){x:=p;observe(&x);if flag{x=q}}
 func fresh(p *int,flag bool){for flag{x:=p;if *p>0{observe(&x)}}}
 func reused(p *int,flag bool){var x *int;for flag{x=p;if *p>0{observe(&x)}}}
 `)
	for _, test := range []struct {
		name    string
		follows bool
		stable  bool
	}{
		{"after", true, false}, {"before", false, true}, {"branch", true, false}, {"fresh", false, true}, {"reused", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			cell := call.Common().Args[0]
			var last *ssa.Store
			for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
				if store.Addr == cell {
					last = store
				}
			}
			if last == nil {
				t.Fatal("missing observed cell store")
			}
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			checkStoreFollowingAllowance(t, cell, call, last, test.follows)
			checkStableContentsAllowance(t, cell, call, test.stable)
		})
	}
}

func checkStoreFollowingAllowance(t *testing.T, cell ssa.Value, call *ssa.Call, last *ssa.Store, want bool) {
	t.Helper()
	cut := proofs.NewSearchBudget(1)
	if StoreMayFollowWithin(cell, call, last, cut) || !cut.Exhausted() {
		t.Fatal("store-order query bypassed caller allowance")
	}
	for limit := 1; limit <= proofs.QueryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		follows := StoreMayFollowWithin(cell, call, last, budget)
		if budget.Exhausted() {
			if follows {
				t.Fatalf("cutoff %d published following store", limit)
			}
			continue
		}
		if follows != want {
			t.Fatalf("following store=%v, want %v", follows, want)
		}
		return
	}
	t.Fatal("store ordering did not recover")
}

func checkStableContentsAllowance(t *testing.T, cell ssa.Value, call *ssa.Call, want bool) {
	t.Helper()
	for limit := 1; limit <= 10*proofs.QueryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		proof := NewStorage(budget).StableContent(cell, call)
		if budget.Exhausted() {
			if proof.Proven() {
				t.Fatalf("cutoff %d published stable contents", limit)
			}
			continue
		}
		if proof.Proven() != want {
			t.Fatalf("stable contents=%+v, want %v", proof, want)
		}
		return
	}
	t.Fatal("stable storage did not recover")
}

func TestStorageStructuralIdentityBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "identity", `package identity
 type owner struct { field int }
 func observe(a,b *int) {}
 func probe(p *owner) { observe(&p.field,&p.field) }
`)
	args := heapObservation(t, pkg.Func("probe")).Common().Args
	if args[0] == args[1] {
		t.Fatal("expected distinct SSA field selections")
	}
	for _, writesOnly := range []bool{false, true} {
		observed := 0
		budget := proofs.NewSearchBudget(1).Observed(func(reason string, _ token.Pos, _ map[string]string) {
			if reason != proofs.EvidenceBudgetExhausted.String() {
				t.Errorf("unexpected cutoff reason: %s", reason)
			}
			observed++
		})
		storage := NewStorage(budget)
		storage.writesOnly = writesOnly
		proof := storage.Same(args[0], args[1])
		if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted || observed != 1 {
			t.Fatalf("cutoff must remain observed unknown: %+v, observed %d", proof, observed)
		}
		fresh := NewStorage(proofs.NewSearchBudget(proofs.QueryBudget))
		fresh.writesOnly = writesOnly
		if !fresh.Same(args[0], args[1]).Proven() || fresh.Budget().Exhausted() {
			t.Fatal("fresh structural equality must retain ordinary and writes-only policy")
		}
	}
	zero := NewStorage(proofs.NewSearchBudget(0))
	if proof := zero.Same(args[0], args[0]); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatal("direct same-value evidence must spend the storage allowance")
	}
	pool := proofs.NewSearchBudget(1)
	shared := pool.Within(proofs.QueryBudget)
	if proof := NewStorage(shared).Same(args[0], args[1]); proof.Proven() || !shared.PoolExhausted() {
		t.Fatal("storage structural queries must retain candidate-pool availability")
	}
}

func TestStorageContentCutoffBeforeGraphFallback(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "content", `package content
 func observe(a,b *int) {}
 func read(p **int) { _ = *p }
 func probe(a *int) { x:=a; read(&x); observe(x,a) }
`)
	call := heapObservation(t, pkg.Func("probe"))
	load, ok := call.Common().Args[0].(*ssa.UnOp)
	if !ok {
		t.Fatal("expected actual captured/addressable cell load")
	}
	cutoff := NewStorage(proofs.NewSearchBudget(1))
	if proof := cutoff.Content(load.X, load); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("graph fallback must not rescue cutoff: %+v", proof)
	}
	fresh := NewStorage(proofs.NewSearchBudget(proofs.QueryBudget))
	if proof := fresh.Content(load.X, load); !proof.Proven() || proof.Value != call.Common().Args[1] || fresh.Budget().Exhausted() {
		t.Fatalf("fresh reaching-write query lost exact contents: %+v", proof)
	}
	// Exhaustion stays authoritative even when a graph was already warmed.
	if !DefinitelySame(load, call.Common().Args[1]) {
		t.Fatal("expected exact identity from the warmed function graph")
	}
	cutoff = NewStorage(proofs.NewSearchBudget(1))
	if proof := cutoff.Content(load.X, load); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatal("cached graph evidence must not override caller availability")
	}
}

func TestReturnedAliasAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnalias", `package returnalias
func returned(value, other *int) (*int, *int) { return other, value }
`)
	fn := pkg.Func("returned")
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	candidates := []ssa.Value{ssa.NewConst(nil, fn.Params[0].Type()), fn.Params[0]}
	for limit := range 7 {
		pool := proofs.NewSearchBudget(limit)
		budget := pool.Within(10)
		got := ReturnedMayAliasAnyWithin(returned, candidates, budget)
		if limit < 6 {
			if got || !budget.Exhausted() {
				t.Fatalf("allowance %d admitted incomplete alias census", limit)
			}
		} else if !got || budget.Exhausted() {
			t.Fatal("fresh complete query failed to recover the returned candidate")
		}
	}
	budget := proofs.NewSearchBudget(10)
	if ReturnedMayAliasAnyWithin(returned, []ssa.Value{ssa.NewConst(nil, fn.Params[0].Type())}, budget) || budget.Exhausted() {
		t.Fatal("unrelated candidate did not complete without alias evidence")
	}
}

func TestDeferredCellRelationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "deferredprobe", `package deferredprobe
type resource struct{}
func (*resource) Close() {}
func register(func()) {}
func exact(p, other *resource, pick bool) { held:=p; defer func(){held.Close()}() }
func cleared(p, other *resource, pick bool) { held:=p; defer func(){if held!=nil{held.Close()}}(); if pick{p.Close();held=nil} }
func replaced(p, other *resource, pick bool) { held:=p; defer func(){held.Close()}(); held=other }
func mixed(p, other *resource, pick bool) { held:=p; defer func(){held.Close()}(); if pick{held=other} }
func aggregate(p, other *resource, pick bool) { held:=[]*resource{p}; defer func(){for _,r:=range held{r.Close()}}(); held=append(held,p) }
func registered(p, other *resource, pick bool) { held:=p; register(func(){held.Close()}) }
`)
	for _, test := range []struct {
		name string
		want DeferredCellMatch
	}{
		{"exact", DeferredCellExact},
		{"cleared", DeferredCellExact},
		{"replaced", DeferredCellUnknown},
		{"mixed", DeferredCellUnknown},
		{"aggregate", DeferredCellContains},
		{"registered", DeferredCellExact},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			cell, invocation := deferredCellCase(t, function)
			full := proofs.NewSearchBudget(proofs.QueryBudget)
			got, known := DeferredCellRelationWithin(cell, function.Params[0], invocation, full)
			if !known || got != test.want || full.Exhausted() {
				t.Fatalf("full relation=%v, known=%v, want %v", got, known, test.want)
			}
			assertDeferredRelationCutoffs(t, cell, function.Params[0], invocation, test.want)
			pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
			cut := pool.Within(1)
			if got, known := DeferredCellRelationWithin(cell, function.Params[0], invocation, cut); known || got != DeferredCellUnknown {
				t.Fatalf("cut published relation=%v, known=%v", got, known)
			}
			if !cut.Exhausted() || pool.Exhausted() {
				t.Fatal("independent child cutoff was not preserved")
			}
			fresh := pool.Within(proofs.QueryBudget)
			if got, known := DeferredCellRelationWithin(cell, function.Params[0], invocation, fresh); !known || got != test.want {
				t.Fatalf("fresh relation=%v, known=%v, want %v", got, known, test.want)
			}
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
		})
	}
}

func assertDeferredRelationCutoffs(t *testing.T, cell *ssa.Alloc, target ssa.Value, invocation ssa.Instruction, want DeferredCellMatch) {
	t.Helper()
	for allowance := 2; allowance < proofs.QueryBudget; allowance++ {
		budget := proofs.NewSearchBudget(allowance)
		got, known := DeferredCellRelationWithin(cell, target, invocation, budget)
		if budget.Exhausted() {
			if known || got != DeferredCellUnknown {
				t.Fatalf("allowance %d published interrupted relation=%v, known=%v", allowance, got, known)
			}
			continue
		}
		if !known || got != want {
			t.Fatalf("completed allowance %d relation=%v, known=%v, want %v", allowance, got, known, want)
		}
		return
	}
	t.Fatal("relation did not complete within query allowance")
}

func deferredCellCase(t *testing.T, function *ssa.Function) (*ssa.Alloc, ssa.Instruction) {
	t.Helper()
	for instruction := range ssaflow.InstructionsWithin(function, nil) {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			continue
		}
		_, closure := ssacall.DirectCallee(common)
		if closure == nil && ssaflow.CallName(common) == "register" {
			closure, _ = common.Args[0].(*ssa.MakeClosure)
		}
		if closure != nil && len(closure.Bindings) == 1 {
			cell, ok := closure.Bindings[0].(*ssa.Alloc)
			if !ok {
				t.Fatalf("captured binding is %T, want actual cell", closure.Bindings[0])
			}
			return cell, instruction
		}
	}
	t.Fatal("missing deferred/callback cell")
	return nil, nil
}

func TestDeferredObservationCensusDiscardsPrefix(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observationprobe", `package observationprobe
func probe(pick bool) { defer func(){}(); if pick{return} }
`)
	function := pkg.Func("probe")
	firstRun, instructions := 0, 0
	for instruction := range ssaflow.InstructionsWithin(function, nil) {
		instructions++
		if _, ok := instruction.(*ssa.RunDefers); ok && firstRun == 0 {
			firstRun = instructions
		}
	}
	if firstRun == 0 || firstRun >= instructions {
		t.Fatal("expected a deferred observation before the completed census")
	}
	cut := proofs.NewSearchBudget(firstRun)
	if points, available := deferredObservationPoints(function, cut); available || len(points) != 0 || !cut.Exhausted() {
		t.Fatalf("cut retained observation prefix: %d points, available=%v", len(points), available)
	}
	if points, available := deferredObservationPoints(function, proofs.NewSearchBudget(proofs.QueryBudget)); !available || len(points) < 2 {
		t.Fatalf("fresh census did not recover: %d points, available=%v", len(points), available)
	}
}
