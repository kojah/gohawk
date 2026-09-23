package ssaflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"

	"golang.org/x/tools/go/ssa"
)

type heapSmokeCase struct {
	name      string
	body      string
	baseline  bool
	prototype bool
}

func heapSmokeCases() []heapSmokeCase {
	return []heapSmokeCase{
		{"direct", `observe(a, a)`, true, true},
		{"cell", `x := a; p := &x; observe(*p, a)`, true, true},
		{"replacement", `x := a; p := &x; *p = b; observe(*p, b)`, true, true},
		{"stale", `x := a; p := &x; *p = b; observe(*p, a)`, false, false},
		{"field", `var x box; x.value = a; observe(x.value, a)`, true, true},
		{"fieldAlias", `var x box; p := &x; x.value = a; p.value = b; observe(x.value, b)`, true, true},
		{"staleField", `var x box; p := &x; x.value = a; p.value = b; observe(x.value, a)`, false, false},
		{"nested", `var x outer; x.inner.value = a; observe(x.inner.value, a)`, true, true},
		{"array", `var x [2]*int; x[0] = a; x[1] = b; observe(x[0], a)`, true, true},
		{"otherIndex", `var x [2]*int; x[0] = a; x[1] = b; observe(x[1], a)`, false, false},
		{"pointerCell", `var x box; p := &x; pp := &p; (*pp).value = a; observe(x.value, a)`, true, true},
		{"snapshot", `var x box; x.value = a; old := x.value; x.value = b; observe(old, a)`, true, true},
		// The graph sees that opaque's empty body keeps nothing.
		{"escape", `var x box; x.value = a; opaque(&x); observe(x.value, a)`, true, false},
		{"dynamicIndex", `var x [2]*int; x[0] = a; x[1] = b; observe(x[idx], a)`, false, false},
		{"branch", `var x box; if pick { x.value = a } else { x.value = a }; observe(x.value, a)`, true, false},
		{"loop", `var x box; for pick { x.value = a }; observe(x.value, a)`, false, false},
		{"aggregateOverwrite", `var x box; x.value = a; x = box{}; observe(x.value, a)`, false, false},
		{"capture", `x := a; keep := func() { _ = x }; _ = keep; observe(x, a)`, true, false},
		{"slice", `x := []*int{a, b}; observe(x[0], a)`, true, false},
	}
}

func buildHeapSmoke(tb testing.TB) *ssa.Package {
	tb.Helper()
	var source strings.Builder
	source.WriteString(`package heapsmoke
type box struct { value *int }
type outer struct { inner box }
func observe(a, b *int) {}
func opaque(any) {}
`)
	for _, test := range heapSmokeCases() {
		source.WriteString("func " + test.name + "(a, b *int, pick bool, idx int) { " + test.body + " }\n")
	}
	return ssaflowtest.BuildPackage(tb, "example.com/heapsmoke", source.String())
}

func heapObservation(tb testing.TB, function *ssa.Function) *ssa.Call {
	tb.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if ok && CallName(call.Common()) == "observe" {
				return call
			}
		}
	}
	tb.Fatal("no observation")
	return nil
}

func TestHeapSmokeComparison(t *testing.T) {
	pkg := buildHeapSmoke(t)
	for _, test := range heapSmokeCases() {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var dump bytes.Buffer
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			call := heapObservation(t, fn)
			args := call.Common().Args
			baseline := NewStorage(NewSearchBudget(1000)).Same(args[0], args[1]).Proven()
			prototype, reason := smokeHeapIdentity(call, 256)
			t.Logf("baseline=%t prototype=%t reason=%s", baseline, prototype, reason)
			if baseline != test.baseline || prototype != test.prototype {
				t.Errorf("got baseline=%t prototype=%t; want baseline=%t prototype=%t", baseline, prototype, test.baseline, test.prototype)
			}
			if proven, _ := smokeHeapIdentity(call, 0); proven {
				t.Fatal("exhausted budget must not prove identity")
			}
		})
	}
}

func BenchmarkHeapSmoke(b *testing.B) {
	pkg := buildHeapSmoke(b)
	call := heapObservation(b, pkg.Func("fieldAlias"))
	args := call.Common().Args
	b.Run("existing", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			NewStorage(NewSearchBudget(1000)).Same(args[0], args[1])
		}
	})
	b.Run("prototype", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			smokeHeapIdentity(call, 256)
		}
	})
}
