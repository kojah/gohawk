package concurrencyfacts

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestEmbeddedFieldBindingKeepsExactAddressPolicy(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fields", `package fields
import "sync"
type Owner struct { mu sync.Mutex }
func helper(owner *Owner) { owner.mu.Lock(); owner.mu.Unlock() }
func existing(owner *Owner) *sync.Mutex { helper(owner); return &owner.mu }
func fresh() *sync.Mutex { owner := new(Owner); helper(owner); return &owner.mu }
func absent(owner *Owner) { helper(owner) }
func changed(cell **Owner, replacement *Owner) *sync.Mutex {
 first := *cell
 helper(first)
 *cell = replacement
 return &(*cell).mu
}
`)
	for _, test := range []struct {
		name     string
		complete bool
	}{
		{"existing", true},
		{"fresh", true},
		{"absent", false},
		{"changed", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			if len(calls) != 1 {
				t.Fatalf("got %d calls, want helper only", len(calls))
			}
			result := NewEngine().AtCall(calls[0], proofs.NewSearchBudget(2000))
			if (result.Reason == ReasonNone) != test.complete {
				t.Fatalf("binding completeness changed: %+v", result)
			}
			if !test.complete {
				return
			}
			if len(result.Operations) != 2 {
				t.Fatalf("lost bound mutex operations: %+v", result)
			}
		})
	}
}

func TestForwardedReceiverFieldsKeepInvocationIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receivers", `package receivers
import "sync"
type owner struct { mu sync.RWMutex }
func (o *owner) run(done chan int) { o.mu.RLock(); close(done); o.mu.RUnlock() }
func (o *owner) Start(done chan int) { go o.run(done) }
func root() {
 var a, b owner
 x, y := make(chan int), make(chan int)
 a.mu.Lock(); b.mu.Lock()
 a.Start(x); b.Start(y)
 <-x; <-y
 a.mu.Unlock(); b.mu.Unlock()
}
`)
	engine := NewEngine()
	result := engine.Root(pkg.Func("root"), proofs.NewSearchBudget(proofs.SummaryBudget))
	if !result.Complete() || len(result.Workers) != 2 {
		t.Fatalf("root = %+v", result)
	}
	for index, worker := range result.Workers {
		if len(worker.Operations) != 3 || worker.Operations[0].Kind != ReadLock || worker.Operations[2].Kind != ReadUnlock {
			t.Fatalf("worker %d = %+v", index, worker)
		}
		if worker.Operations[0].Resource != result.Operations[index].Resource {
			t.Errorf("worker %d bound to a different receiver", index)
		}
	}
	if result.Workers[0].Operations[0].Resource == result.Workers[1].Operations[0].Resource {
		t.Error("two invocations shared a receiver identity")
	}
	// The launch method does not itself select mu. Its field relation must
	// still be publishable relative to its receiver, without an invented SSA cell.
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("root")) {
		function := call.Common().StaticCallee()
		if function != nil && function.Name() == "Start" {
			assertReceiverWorkerFact(t, engine, function)
		}
	}
}

func assertReceiverWorkerFact(t *testing.T, engine *Engine, function *ssa.Function) {
	t.Helper()
	declaration := engine.Function(function, proofs.NewSearchBudget(proofs.SummaryBudget))
	fact, exported := exportSummary(function, declaration, nil)
	if !exported || len(fact.Workers) != 1 || len(fact.Workers[0].Effects) != 3 {
		t.Fatalf("receiver declaration was not exported: %+v (%+v)", fact, declaration)
	}
	first := fact.Workers[0].Effects[0]
	if first.Parameter != 0 || first.Kind != ReadLock || len(first.Fields) != 1 || first.Fields[0] != 0 {
		t.Errorf("exported field relation = %+v", first)
	}
}

func TestReceiverChannelRelationships(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "owners", `package owners
import "sync"
type owner struct { mu sync.Mutex; done chan int }
func (o *owner) run() { o.mu.Lock(); close(o.done); o.mu.Unlock() }
func (o *owner) start() { go o.run() }
func (o *owner) stop() { <-o.done; o.mu.Unlock() }
func root() { var o owner; o.done = make(chan int); o.mu.Lock(); o.start(); o.stop() }
func mutable() { var o owner; o.done = make(chan int); o.mu.Lock(); o.start(); o.done = make(chan int); o.stop() }
func external(o *owner) { o.mu.Lock(); o.start(); o.stop() }
func pair() {
 var a, b owner
 a.done = make(chan int); b.done = make(chan int)
 a.mu.Lock(); b.mu.Lock(); a.start(); b.start(); a.stop(); b.stop()
}
`)
	for _, name := range []string{"mutable", "external"} {
		got := NewEngine().Root(pkg.Func(name), proofs.NewSearchBudget(proofs.SummaryBudget))
		if got.Complete() {
			t.Errorf("%s accepted unstable channel identity: %+v", name, got)
		}
	}
	got := NewEngine().Root(pkg.Func("root"), proofs.NewSearchBudget(proofs.SummaryBudget))
	if !got.Complete() || len(got.Workers) != 1 || len(got.Operations) != 3 {
		t.Fatalf("receiver channel protocol = %+v", got)
	}
	if got.Workers[0].Operations[1].Resource != got.Operations[1].Resource {
		t.Error("worker completion and parent wait disagree on channel identity")
	}
	pair := NewEngine().Root(pkg.Func("pair"), proofs.NewSearchBudget(proofs.SummaryBudget))
	if !pair.Complete() || len(pair.Workers) != 2 || len(pair.Operations) != 6 {
		t.Fatalf("two-owner protocol = %+v", pair)
	}
	for index, worker := range pair.Workers {
		if worker.Operations[1].Resource != pair.Operations[2+2*index].Resource {
			t.Errorf("owner %d completion was rebound to another invocation", index)
		}
	}
	if pair.Workers[0].Operations[1].Resource == pair.Workers[1].Operations[1].Resource {
		t.Error("separate owners shared a channel identity")
	}
}

const spillPathFixture = `package spillpaths
 import "sync"
 type D struct{mu sync.Mutex}
 func early(d *D){var p *D;before:=p;p=d;f:=func(){println(p)};f();before.mu.Lock()}
 func after(d *D){var p *D;p=d;f:=func(){println(p)};f();p.mu.Lock()}
 func entry(d *D){f:=func(){println(d)};f();d.mu.Lock()}
 func reassigns(d,o *D){before:=d;f:=func(){println(d)};f();d=o;before.mu.Lock();d.mu.Lock()}
 type child struct{mu sync.Mutex}
 type owner struct{c *child}
 func newOwner()*owner{return &owner{c:&child{}}}
 func field(o *owner){o.c.mu.Lock();o.c.mu.Unlock()}
`

func TestSpillPathsRetainReadTimeIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spillpaths", spillPathFixture)
	for _, test := range []struct {
		name       string
		parameters []int
	}{
		{"early", []int{-1}}, {"after", []int{0}}, {"entry", []int{0}}, {"reassigns", []int{0, 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			fields := ssaflow.InstructionsOf[*ssa.FieldAddr](fn)
			if len(fields) != len(test.parameters) {
				t.Fatalf("fields=%d", len(fields))
			}
			for index, field := range fields {
				want := test.parameters[index]
				path, ok := embeddedPathWithin(field, nil)
				if ok != (want >= 0) {
					t.Fatalf("path=%+v known=%v want parameter%d", path, ok, want)
				}
				if ok && path.Root != fn.Params[want] {
					t.Fatalf("path root=%v want%v", path.Root, fn.Params[want])
				}
				checkSpillPathCutoffs(t, field, path, ok)
			}
		})
	}
}

func checkSpillPathCutoffs(t *testing.T, field *ssa.FieldAddr, want ssaflow.EmbeddedFieldPath, known bool) {
	t.Helper()
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got, ok := embeddedPathWithin(field, budget)
		if budget.Exhausted() {
			if ok {
				t.Fatalf("cut%d retained%+v", limit, got)
			}
			continue
		}
		if ok != known || ok && got != want {
			t.Fatalf("complete%d=%+v/%v want%+v/%v", limit, got, ok, want, known)
		}
		return
	}
	t.Fatal("spill query never completed")
}

func TestCanonicalFieldCutoffDoesNotPoisonFreshQuery(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "spillpaths", spillPathFixture).Func("field")
	loads := ssaflow.InstructionsOf[*ssa.UnOp](fn)
	if len(loads) != 2 {
		t.Fatalf("loads=%d", len(loads))
	}
	engine := NewEngine()
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(0)
	if _, ok := engine.query(child).fixedLoad(loads[0]); ok || !child.Exhausted() || pool.Exhausted() {
		t.Fatal("expected child cutoff")
	}
	if _, cached := engine.fields.canonical[loads[0]]; cached {
		t.Fatal("interrupted canonical sentinel retained")
	}
	first, ok := engine.query(pool.Within(proofs.SummaryBudget)).fixedLoad(loads[0])
	if !ok || first != loads[0] {
		t.Fatalf("fresh=%v/%v", first, ok)
	}
	second, ok := engine.query(pool.Within(proofs.SummaryBudget)).fixedLoad(loads[1])
	if !ok || second != first {
		t.Fatalf("canonical second=%v/%v", second, ok)
	}
}

func TestCapturedFieldReadCensusCutoff(t *testing.T) {
	source := `package capturereads
 import "sync"
 type D struct{mu sync.Mutex}
 func subject(d *D){go func(){` + strings.Repeat("println(d);", proofs.QueryBudget+1) + `d.mu.Lock()}()}
 `
	fn := ssaflowtest.BuildPackage(t, "capturereads", source).Func("subject").AnonFuncs[0]
	loads := ssaflow.InstructionsOf[*ssa.UnOp](fn)
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	got, ok := NewEngine().query(child).fixedLoad(loads[0])
	if ok || got != nil || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("capture cut=%v/%v", got, ok)
	}
	fresh, ok := NewEngine().query(pool.Within(proofs.SummaryBudget)).fixedLoad(loads[0])
	if !ok || fresh != loads[0] {
		t.Fatalf("fresh capture=%v/%v", fresh, ok)
	}
}

func TestSpillOrderingSharesPathAllowance(t *testing.T) {
	source := `package spillorder
 import "sync"
 type D struct{mu sync.Mutex;n int}
 func padded(d *D)int{f:=func(){_=d.n};_=f;n:=0;` + strings.Repeat("n++;", proofs.QueryBudget+1) + `d.mu.Lock();d.mu.Unlock();return n}
 `
	fn := ssaflowtest.BuildPackage(t, "spillorder", source).Func("padded")
	field := ssaflow.InstructionsOf[*ssa.FieldAddr](fn)[0]
	pool := proofs.NewSearchBudget(20 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	if path, ok := embeddedPathWithin(field, child); ok || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("path cut=%+v/%v", path, ok)
	}
	path, ok := embeddedPathWithin(field, pool.Within(2*proofs.SummaryBudget))
	if !ok || path.Root != fn.Params[0] {
		t.Fatalf("fresh path=%+v/%v", path, ok)
	}
	engine := NewEngine()
	short := pool.Within(proofs.SummaryBudget)
	if result := engine.Function(fn, short); result.Complete() || !short.Exhausted() {
		t.Fatalf("summary cutoff=%+v", result)
	}
	fresh := engine.Function(fn, pool.Within(10*proofs.SummaryBudget))
	if !pairedLock(fresh) {
		t.Fatalf("fresh summary=%+v", fresh)
	}
}

func TestFieldLoadDiscoveryPreservesPathOrderAndCutoffs(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fielddiscovery", `package fielddiscovery
 import "sync"
 type child struct{mu sync.Mutex}
 type owner struct{c,d *child}
 func newOwner()*owner{return &owner{c:&child{},d:&child{}}}
 func mixed(a,b *owner){println(b.c);println(a.d);println(a.c);println(a.c)}
 `)
	fn := pkg.Func("mixed")
	loads := ssaflow.InstructionsOf[*ssa.UnOp](fn)
	if len(loads) != 4 {
		t.Fatalf("loads=%d", len(loads))
	}
	address := loads[2].X.(*ssa.FieldAddr)
	path, ok := embeddedPathWithin(address, nil)
	if !ok {
		t.Fatal("field has no exact path")
	}
	for _, mode := range []string{"caller", "canonical"} {
		t.Run(mode, func(t *testing.T) {
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				engine := NewEngine()
				pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
				child := pool.Within(limit)
				query := engine.query(child)
				var got *ssa.UnOp
				var known bool
				if mode == "caller" {
					got, known = query.callerLoad(fn, path, fieldOf(address))
				} else {
					got, known = query.fixedLoad(loads[3])
				}
				if child.Exhausted() {
					if known || got != nil || pool.Exhausted() {
						t.Fatalf("cutoff %d: %v/%v", limit, got, known)
					}
					query = engine.query(pool.Within(proofs.SummaryBudget))
					if mode == "caller" {
						got, known = query.callerLoad(fn, path, fieldOf(address))
					} else {
						got, known = query.fixedLoad(loads[3])
					}
					if !known || got != loads[2] {
						t.Fatalf("fresh after cutoff %d: %v/%v", limit, got, known)
					}
					continue
				}
				if !known || got != loads[2] {
					t.Fatalf("complete allowance %d: %v/%v", limit, got, known)
				}
				return
			}
			t.Fatal("field load query never completed")
		})
	}
}
