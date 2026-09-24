package heapmodel

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// Dispatch identity and capture identity are separate from the callee's
// effects. Unknown alternatives must not acquire an unconditional write.
func TestHeapCallBindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heapbindings", `package heapbindings
type box struct { value *int }
type setter interface { Set(*int) }
func (b *box) Set(p *int) { b.value = p }
func observe(a,b any) {}
func opaque(*box)
func exactClosure(p *box, q *int) { func() { p.value=q }(); observe(p.value,q) }
func conditionalClosure(p *box, q *int, yes bool) { func() { if yes { p.value=q } }(); observe(p.value,q) }
func opaqueClosure(p *box, q *int) { func() { opaque(p) }(); observe(p.value,q) }
func exactInterface(p *box, q *int) { var s setter=p; s.Set(q); observe(p.value,q) }
func ambiguousInterface(p,r *box, q *int, yes bool) { var s setter=p; if yes { s=r }; s.Set(q); observe(p.value,q) }
func foreignInterface(s setter,p *box,q *int) { s.Set(q); observe(p.value,q) }
func changedCapture(p,r *box,q *int) { f:=func() { p.value=q }; p=r; f(); observe(r.value,q) }
func uncertainCapture(p,r *box,q *int, yes bool) { f:=func() { p.value=q }; if yes { p=r }; f(); observe(p.value,q) }
func writesCapture(p,r *box,q *int) { func() { p=r; p.value=q }(); observe(r.value,q) }
func closureArgument(p *box,q *int) { func(v *int) { p.value=v }(q); observe(p.value,q) }
func closureResult(p *box) { r:=func() *box { return p }(); observe(r,p) }
func recursiveCapture(p *box,q *int) { var f func(); f=func() { f(); p.value=q }; f(); observe(p.value,q) }
func nestedPointer(p *box,q *int) { pp:=&p; func() { (*pp).value=q }(); observe(p.value,q) }
func deferredExact(p *box,q *int) { defer func() { p.value=q }() }
func deferredCapture(p,r *box,q *int) { defer func() { p.value=q }(); p=r }
func deferredConditional(p *box,q *int, yes bool) { if yes { defer func() { p.value=q }() } }
func deferredOrder(p *box,a,b *int) { defer func() { p.value=a }(); defer func() { p.value=b }() }
func deferredLoop(p *box,q *int,n int) { for i:=0;i<n;i++ { defer func() { p.value=q }() } }
func directDeferredConditional(p *box,q *int, yes bool) { if yes { defer p.Set(q) } }
func directDeferredOrder(p *box,a,b *int) { defer p.Set(a); defer p.Set(b) }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"exactClosure", true},
		{"conditionalClosure", false},
		{"opaqueClosure", false},
		{"exactInterface", true},
		{"ambiguousInterface", false},
		{"foreignInterface", false},
		{"changedCapture", true},
		{"uncertainCapture", false},
		{"writesCapture", false},
		{"closureArgument", true},
		{"closureResult", true},
		{"recursiveCapture", false},
		{"nestedPointer", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var dump bytes.Buffer
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			before := map[*ssa.Function]string{}
			for _, child := range function.AnonFuncs {
				summary, _ := ProjectHeap(child)
				before[child] = summary.String()
				t.Log(summary.String())
			}
			call := heapObservation(t, function)
			left, right := unwrapInterface(call.Common().Args[0]), unwrapInterface(call.Common().Args[1])
			if got := regionsOfFunction(function).mustSame(left, right); got != test.want {
				t.Fatalf("mustSame = %t, want %t\n%s", got, test.want, RenderRegions(function))
			}
			for child, previous := range before {
				summary, _ := ProjectHeap(child)
				if summary.String() != previous {
					t.Fatal("call substitution mutated the shared callee summary")
				}
			}
		})
	}
	for _, test := range []struct {
		name   string
		want   string
		absent string
	}{
		{"deferredExact", "edge P0/field:0 -> P1 must", ""},
		{"deferredCapture", "", "edge P1/field:0 -> P2 must"},
		{"deferredConditional", "", "edge P0/field:0 -> P1 must"},
		{"deferredOrder", "edge P0/field:0 -> P1 must", "edge P0/field:0 -> P2 must"},
		{"deferredLoop", "", "edge P0/field:0 -> P1 must"},
		{"directDeferredConditional", "", "edge P0/field:0 -> P1 must"},
		{"directDeferredOrder", "edge P0/field:0 -> P1 must", "edge P0/field:0 -> P2 must"},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(test.name))
			if !ok {
				t.Fatal("summary unavailable")
			}
			rendered := summary.String()
			if test.want != "" && !strings.Contains(rendered, test.want) || test.absent != "" && strings.Contains(rendered, test.absent) {
				t.Fatalf("unexpected deferred summary:\n%s\n%s", rendered, RenderRegions(pkg.Func(test.name)))
			}
		})
	}
}

func TestHeapBindingPreservesDeclarationIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heapdeclarations", `package heapdeclarations
import "errors"
func identity[T any](p *T) *T { return p }
func generic(p *int) *int { return identity(p) }
func imported() error { return errors.New("failure") }
`)
	for _, name := range []string{"generic", "imported"} {
		call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name))[0]
		callee := call.Common().StaticCallee()
		if name == "generic" && callee.Origin() == nil {
			t.Fatal("fixture did not create a generic instantiation")
		}
		if name == "imported" && len(callee.Blocks) != 0 {
			t.Fatal("fixture did not create an imported declaration")
		}
		binding, reason := resolveHeapCall(call.Common(), call)
		if reason != CallSummaryApplied || binding.callee != callee || len(binding.args) != 1 {
			t.Fatalf("%s: lost concrete callee or signature binding: %+v %s", name, binding, reason)
		}
	}
}
