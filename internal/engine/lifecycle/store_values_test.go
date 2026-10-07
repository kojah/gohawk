package lifecycle

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func storedOwnerFixture(t *testing.T) (*ssa.Function, ssa.Value) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "storedowner", `package storedowner
type owner struct { direct *int; indexed [2]*int; slot **int; next *owner }
func aggregate(value, other *int) *owner {
	box := new(owner)
	box.direct = value
	box.indexed[1] = other
	box.slot = new(*int)
	*box.slot = value
	box.next = box
	return box
}
`)
	fn := pkg.Func("aggregate")
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			t.Log(instruction.String())
		}
	}
	return fn, ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
}

func TestStoredValueTraversalAllowance(t *testing.T) {
	fn, root := storedOwnerFixture(t)
	var complete []ssa.Value
	for value := range StoredInto(root) {
		complete = append(complete, value)
	}
	counts := map[ssa.Value]int{}
	for _, value := range complete {
		counts[value]++
	}
	if len(complete) != 5 || counts[fn.Params[0]] != 2 || counts[fn.Params[1]] != 1 || counts[root] != 1 {
		t.Fatalf("field, index, loaded slot and cyclic owner contents = %v", counts)
	}
	finished := false
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		pool := proofs.NewSearchBudget(limit)
		budget := pool.Within(proofs.SummaryBudget)
		got := map[ssa.Value]int{}
		for value := range StoredIntoWithin(root, budget) {
			got[value]++
		}
		if budget.Exhausted() {
			for value, count := range got {
				if count > counts[value] {
					t.Fatalf("cutoff invented stored value %v", value)
				}
			}
			continue
		}
		if limit == 0 || len(got) != len(counts) {
			t.Fatalf("complete traversal = %v, want %v", got, counts)
		}
		for value, count := range counts {
			if got[value] != count {
				t.Fatalf("complete traversal lost %v: %v", value, got)
			}
		}
		finished = true
		break
	}
	if !finished {
		t.Fatal("stored-value traversal never completed")
	}
}

func TestStoredValueTraversalEarlyStop(t *testing.T) {
	_, root := storedOwnerFixture(t)
	budget := proofs.NewSearchBudget(proofs.SummaryBudget)
	visits := 0
	for range StoredIntoWithin(root, budget) {
		visits++
		break
	}
	if visits != 1 || budget.Exhausted() {
		t.Fatal("early stopping must complete one yield without exhausting")
	}
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	child := pool.Within(proofs.SummaryBudget)
	sibling := pool.Within(proofs.SummaryBudget)
	visits = 0
	for range StoredIntoWithin(root, child) {
		visits++
		for sibling.Spend() {
		}
	}
	if visits != 1 || !child.PoolExhausted() {
		t.Fatal("callback pool cutoff must stop further yields")
	}
}

func TestStoredValueTraversalOpaqueForms(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storedopaque", `package storedopaque
type slot **int
func hidden(box **int, value *int) { *box = value }
func called(value *int) **int { box := new(*int); hidden(box, value); return box }
func converted(value *int) **int { box := new(*int); *slot(box) = value; return box }
func merged(value *int, flag bool) **int {
	box := new(*int)
	x := box
	if flag { x = new(*int) }
	*x = value
	return box
}
`)
	for _, name := range []string{"called", "converted", "merged"} {
		fn := pkg.Func(name)
		root := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
		budget := proofs.NewSearchBudget(proofs.SummaryBudget)
		for value := range StoredIntoWithin(root, budget) {
			t.Fatalf("%s crossed an opaque call, conversion or phi to %v", name, value)
		}
		if budget.Exhausted() {
			t.Fatalf("%s failed to complete the bounded query", name)
		}
	}
}

// Both queries follow forwarding forms, but only general transfer follows
// reads from a local cell. A return or opaque use is not a field store.
func TestForwardUseTransferBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type holder struct { value *int; boxed any }
var global *int
func consume(*int)
func identity(p *int) *int { return p }
func pair(p *int) (*int, bool) { return p, false }
func directField(p *int, h *holder) { h.value = identity(p) }
func boxedField(p *int, h *holder) { h.boxed = identity(p) }
func tupleField(p *int, h *holder) { h.value, _ = pair(p) }
func returned(p *int) *int { return identity(p) }
func globalStore(p *int) { global = identity(p) }
func opaqueUse(p *int) { consume(identity(p)) }
func localCell(p *int, h *holder) {
	x := identity(p)
	func() { _ = x }()
	h.value = x
}
func localCycle(p *int, n int) {
	x := identity(p)
	for i := 0; i < n; i++ {
		consume(x)
		if i%2 == 0 { x = identity(nil) }
	}
}
func returnedCycle(p *int, n int) *int {
	x := identity(p)
	for i := 0; i < n; i++ { if i%2 == 0 { x = identity(nil) } }
	return x
}
`)
	for _, test := range []struct {
		name            string
		field, transfer bool
	}{
		{"directField", true, true},
		{"boxedField", true, true},
		{"tupleField", true, true},
		{"returned", false, true},
		{"globalStore", false, false},
		{"opaqueUse", false, false},
		{"localCell", false, true},
		{"localCycle", false, false},
		{"returnedCycle", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			if len(calls) == 0 {
				t.Fatal("missing constructor call")
			}
			call := calls[0]
			if got := CallTransfersValueToField(call, function.Params[0]); got != test.field {
				t.Errorf("field transfer = %t, want %t", got, test.field)
			}
			if got := ValueHasTransferUse(call); got != test.transfer {
				t.Errorf("general transfer = %t, want %t", got, test.transfer)
			}
		})
	}
}

func TestFluentTransferPreservesReceiverType(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type owner struct { p *int }
func (o *owner) Next() *owner { return o }
func (o *owner) Read() *int { return o.p }
func retained(o *owner) *owner { return o.Next() }
func borrowed(o *owner) *int { return o.Read() }
func local(o *owner) { o.Next() }
`)
	for name, want := range map[string]bool{"retained": true, "borrowed": false, "local": false} {
		if got := ValueHasTransferUse(pkg.Func(name).Params[0]); got != want {
			t.Errorf("%s: receiver transfer = %t, want %t", name, got, want)
		}
	}
}

func TestCallConsumptionAliasBoundary(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type holder struct{ptr *int;result *int}
 func identity(a,b *int)*int{return b}
 func keepHolder(h *holder)*int{return h.ptr}
 func cleanup(a,b *int)func(){return func(){println(b)}}
 func cleanupHolder(h *holder)func(){return func(){println(h.ptr)}}
 func fieldLate(){p:=new(int);q:=new(int);h:=new(holder);h.result=identity(q,p);println(p,h)}
 func fieldOther(){p:=new(int);q:=new(int);h:=new(holder);h.result=identity(q,q);println(p,h)}
 func fieldContained(){p:=new(int);h:=new(holder);h.result=keepHolder(&holder{ptr:p});println(p,h)}
 func deferredLate(){p:=new(int);q:=new(int);defer cleanup(q,p)();println(p)}
 func deferredOther(){p:=new(int);q:=new(int);defer cleanup(q,q)();println(p)}
 func deferredContained(){p:=new(int);defer cleanupHolder(&holder{ptr:p})();println(p)}
 `)
	for _, test := range []struct {
		name        string
		field, want bool
	}{
		{"fieldLate", true, true},
		{"fieldOther", true, false},
		{"fieldContained", true, false},
		{"deferredLate", false, true},
		{"deferredOther", false, false},
		{"deferredContained", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			target := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			var got bool
			if test.field {
				got = CallTransfersValueToField(call, target)
			} else {
				got = CallReturnsDeferredCleanup(call, target)
			}
			if got != test.want {
				t.Fatalf("consumption=%v want %v", got, test.want)
			}
		})
	}
}

func TestStoredCallbackOwnershipBoundary(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type resource struct{ value int }
type holder struct { callback func(); value *resource; nested *holder }
func direct(target *holder, value, other *resource) {
	target.callback = func() { _ = value }
}
func unrelated(target *holder, value, other *resource) {
	target.callback = func() { _ = other }
}
func nestedCallback(target *holder, value, other *resource) {
	callback := func() { _ = value }
	target.callback = func() { callback() }
}
func capturedAggregate(target *holder, value, other *resource) {
	owner := &holder{value: value}
	target.callback = func() { _ = owner.value }
}
func storedAggregate(target *holder, value, other *resource) {
	target.nested = &holder{value: value}
}
`)
	for _, test := range []struct {
		name              string
		callback, broader bool
	}{
		{"direct", true, true},
		{"unrelated", false, false},
		{"nestedCallback", true, true},
		{"capturedAggregate", false, true},
		{"storedAggregate", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				store, ok := instruction.(*ssa.Store)
				if !ok {
					return false
				}
				field, ok := store.Addr.(*ssa.FieldAddr)
				return ok && field.X == function.Params[0]
			})
			value := function.Params[1]
			if owns := StoresOwnerOfValueInField(instruction, value); owns != test.callback {
				t.Errorf("StoresOwnerOfValueInField = %t, want %t", owns, test.callback)
			}
			if contains := MayContainValue(instruction.(*ssa.Store).Val, value); contains != test.broader {
				t.Errorf("MayContainValue = %t, want %t", contains, test.broader)
			}
		})
	}
}

func TestReturnedConstructorCallBinding(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "constructorbinding", `package constructorbinding
type owner struct { value *int }
type builder struct {}
func (*builder) Build(p *int) *owner { return &owner{p} }
type factory interface { Build(*int) *owner }
func direct(p *int) *owner { return (&builder{}).Build(p) }
func dynamic(p *int, f factory) *owner { return f.Build(p) }
func boxed(p *int) *owner { var f factory = &builder{}; return f.Build(p) }
`)
	for _, name := range []string{"direct", "dynamic", "boxed"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			t.Log(call.String())
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			if name == "direct" {
				if call.Common().IsInvoke() || len(call.Common().Args) != 2 || call.Common().Args[1] != fn.Params[0] {
					t.Fatal("direct method fixture lacks separate receiver and resource operands")
				}
			} else if !call.Common().IsInvoke() || call.Common().StaticCallee() != nil {
				t.Fatal("interface fixture no longer has unresolved dispatch")
			}
			proof := ProveReturnedOwnershipWithin(returned, fn.Params[0], nil, proofs.NewSearchBudget(proofs.SummaryBudget))
			if proof.State == proofs.EvidenceUnknown || proof.Proven() != (name == "direct") {
				t.Fatalf("constructor binding = %+v", proof)
			}
			// A completed decline supplies no owner evidence, not a claim
			// that a dynamically dispatched constructor cannot retain its input.
			cutoff := ProveReturnedOwnershipWithin(returned, fn.Params[0], nil, proofs.NewSearchBudget(0))
			if cutoff.State != proofs.EvidenceUnknown || cutoff.Reason != proofs.EvidenceBudgetExhausted {
				t.Fatalf("interrupted constructor binding = %+v", cutoff)
			}
		})
	}
}

func TestObservedContainmentAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observedowner", `package observedowner
type holder struct { value *int }
func observe(any)
func direct(p, other *int) { observe(p) }
func nested(p, other *int) { observe(&holder{p}) }
func unrelated(p, other *int) { observe(&holder{other}) }
func closure(p, other *int) { observe(func(){ println(p) }) }
func fill(o *holder, p *int) { o.value = p }
func later(p, other *int) { o := &holder{}; observe(o); fill(o,p) }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"direct", true},
		{"nested", true},
		{"unrelated", false},
		{"closure", true},
		// The existing structural may-search includes later visible stores.
		// Only the graph fallback is observed; this cannot establish cleanup.
		{"later", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			owner := call.Common().Args[0]
			for _, at := range []ssa.Instruction{call, nil} {
				baseline := ProveMayContainValueAtWithin(owner, fn.Params[0], at, nil).Proven()
				if baseline != test.want {
					t.Fatalf("default containment = %v, want %v", baseline, test.want)
				}
				completed := false
				for limit := 0; limit <= proofs.SummaryBudget; limit++ {
					pool := proofs.NewSearchBudget(limit)
					budget := pool.Within(proofs.SummaryBudget)
					got := ProveMayContainValueAtWithin(owner, fn.Params[0], at, budget)
					if budget.Exhausted() || budget.PoolExhausted() {
						if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
							t.Fatalf("allowance %d retained interrupted containment: %+v", limit, got)
						}
						continue
					}
					if limit == 0 || got.State == proofs.EvidenceUnknown || got.Proven() != baseline {
						t.Fatalf("complete containment = %+v, want %v", got, baseline)
					}
					completed = true
					break
				}
				if !completed {
					t.Fatal("containment never completed")
				}
			}
		})
	}
}

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
