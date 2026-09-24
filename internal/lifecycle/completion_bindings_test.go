package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"

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
				Methods: []string{test.method}, Budget: ssaflow.NewSearchBudget(1000),
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
					Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(1000),
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
		Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(1000),
	})
	if proof.State != ssaflow.EvidenceUnknown {
		t.Fatalf("unresolved callback = %+v", proof)
	}
}

func TestCallbackBindingsBudget(t *testing.T) {
	pkg := buildTestSSA(t, callbackBindingsFixture)
	fn := pkg.Func("forwarded")
	proof := ProveCompletion(CompletionRequest{
		Instruction: findLaunch(t, fn), Target: fn.Params[0],
		Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(1),
	})
	if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted {
		t.Fatalf("exhausted callback search = %+v", proof)
	}
}
