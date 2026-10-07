package ssaflow_test

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// WrittenOnceCell accepts a cell whose one store is its only write anywhere,
// and rejects every other shape by the cause that could change what a read
// returns.
func TestWrittenOnceCellRejectionsByCause(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cells", writtenOnceCellFixture)
	for name, want := range map[string]bool{
		"readOnly":               true,
		"storedTwice":            false,
		"writtenInClosure":       false,
		"writtenInNestedClosure": false,
		"addressTaken":           false,
		"neverStored":            false,
	} {
		allocs := ssaflow.InstructionsOf[*ssa.Alloc](pkg.Func(name))
		if len(allocs) != 1 {
			t.Fatalf("%s: %d cells", name, len(allocs))
		}
		stored, once := ssaflow.WrittenOnceCellWithin(allocs[0], nil)
		if once != want {
			t.Errorf("%s: written once = %t, want %t", name, once, want)
		}
		if once && stored != pkg.Func(name).Params[0] {
			t.Errorf("%s: stored value %v, want the parameter", name, stored)
		}
	}
}

// Each reaching fold looks through exactly the transparent forms it names:
// one form reaches the wrapped value, no forms stops at the wrapper, and a
// call that returns its argument is never a form.
func TestReachingWalkTransparentForms(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "forms", `package forms
type Named *int
type I interface{ M(); N() }
type J interface{ M() }
type T int
func (T) M() {}
func (T) N() {}
func identity(p *int) *int { return p }
func sinkI(I)      {}
func sinkJ(J)      {}
func sinkNamed(Named) {}
func sinkWide(int64)  {}
func sinkT(T)      {}
func sinkPointer(*int) {}
func makeInterface(t T)     { var i I = t; sinkI(i) }
func changeInterface(i I)   { var j J = i; sinkJ(j) }
func changeType(p *int)     { sinkNamed(Named(p)) }
func convert(n int)         { sinkWide(int64(n)) }
func typeAssert(i I)        { sinkT(i.(T)) }
func opaque(p *int)         { sinkPointer(identity(p)) }
`)
	for _, test := range []struct {
		name string
		form ssaflow.TransparentValueForm
	}{
		{"makeInterface", ssaflow.TransparentMakeInterface},
		{"changeInterface", ssaflow.TransparentChangeInterface},
		{"changeType", ssaflow.TransparentChangeType},
		{"convert", ssaflow.TransparentConvert},
		{"typeAssert", ssaflow.TransparentTypeAssert},
		{"opaque", ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert |
			ssaflow.TransparentMakeInterface | ssaflow.TransparentTypeAssert},
	} {
		function := pkg.Func(test.name)
		parameter := function.Params[0]
		wrapped := sunkValue(t, function)
		isParameter := func(_ ssaflow.ReachingWalk, value ssa.Value) bool { return value == parameter }
		want := test.name != "opaque"
		if got := ssaflow.NewReachingWalk(test.form).Any(wrapped, isParameter); got != want {
			t.Errorf("%s: Any through its form = %t, want %t", test.name, got, want)
		}
		if got := ssaflow.NewReachingWalk(test.form).Every(wrapped, isParameter); got != want {
			t.Errorf("%s: Every through its form = %t, want %t", test.name, got, want)
		}
		if ssaflow.NewReachingWalk(ssaflow.TransparentNone).Any(wrapped, isParameter) {
			t.Errorf("%s: a fold with no forms looked through the wrapper", test.name)
		}
		if test.name != "opaque" && test.form != ssaflow.TransparentMakeInterface &&
			ssaflow.NewReachingWalk(ssaflow.TransparentMakeInterface).Any(wrapped, isParameter) {
			t.Errorf("%s: a fold naming only another form looked through it", test.name)
		}
	}
}

// sunkValue returns the argument of the function's one call to a sink.
func sunkValue(t *testing.T, function *ssa.Function) ssa.Value {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		if name := ssaflow.CallName(call.Common()); len(name) > 4 && name[:4] == "sink" {
			return call.Common().Args[0]
		}
	}
	t.Fatalf("%s: no call to a sink", function.Name())
	return nil
}

const writtenOnceCellFixture = `package cells
func escape(*int) {}
func readOnly(n int) func() int {
	v := n
	return func() int { return v }
}
func storedTwice(n, m int) func() int {
	v := n
	v = m
	return func() int { return v }
}
func writtenInClosure(n int) func() int {
	v := n
	return func() int { v++; return v }
}
type bump func()
func writtenInNestedClosure(n int) func() bump {
	v := n
	return func() bump { return func() { v = v + 1 } }
}
func addressTaken(n int) func() int {
	v := n
	escape(&v)
	return func() int { return v }
}
func neverStored() func() int {
	var v int
	return func() int { return v }
}
`

func TestWrittenOnceCellAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cells", writtenOnceCellFixture)
	for _, name := range []string{"readOnly", "storedTwice", "writtenInClosure", "writtenInNestedClosure", "addressTaken", "neverStored"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			cell := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
			want, once := ssaflow.WrittenOnceCellWithin(cell, nil)
			completed := false
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got, ok := ssaflow.WrittenOnceCellWithin(cell, budget)
				if budget.Exhausted() || limit == 0 {
					if got != nil || ok {
						t.Fatalf("cut%d published stored value %v", limit, got)
					}
					continue
				}
				if got != want || ok != once {
					t.Fatalf("complete=%v/%v want%v/%v", got, ok, want, once)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("cell census never completed")
			}
			pool := proofs.NewSearchBudget(proofs.SummaryBudget)
			if got, ok := ssaflow.WrittenOnceCellWithin(cell, pool.Within(1)); got != nil || ok || pool.Exhausted() {
				t.Fatalf("child=%v/%v pool exhausted=%v", got, ok, pool.Exhausted())
			}
			if got, ok := ssaflow.WrittenOnceCellWithin(cell, pool.Within(proofs.QueryBudget)); got != want || ok != once {
				t.Fatalf("fresh=%v/%v", got, ok)
			}
		})
	}
}

func TestWrittenOnceNestedReaderAllowance(t *testing.T) {
	source := `package manyreads
 func read(n int)func(){v:=n;return func(){` + strings.Repeat("println(v);", proofs.QueryBudget+1) + `}}
 `
	fn := ssaflowtest.BuildPackage(t, "manyreads", source).Func("read")
	cell := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	if stored, ok := ssaflow.WrittenOnceCellWithin(cell, child); stored != nil || ok || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("nested reader cut=%v/%v, child exhausted=%v, parent exhausted=%v", stored, ok, child.Exhausted(), pool.Exhausted())
	}
	fresh := pool.Within(2 * proofs.SummaryBudget)
	if stored, ok := ssaflow.WrittenOnceCellWithin(cell, fresh); !ok || stored != fn.Params[0] {
		t.Fatalf("fresh nested reader=%v/%v", stored, ok)
	}
}
