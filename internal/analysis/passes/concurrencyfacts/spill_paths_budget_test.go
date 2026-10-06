package concurrencyfacts

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
