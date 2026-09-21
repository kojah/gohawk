package ssaflow

import "testing"

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
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			proof := ProveCompletion(CompletionRequest{
				Instruction: findLaunch(t, fn), Target: fn.Params[0],
				Methods: []string{test.method}, Budget: NewSearchBudget(1000),
			})
			if proof.Proven() != test.proven {
				t.Fatalf("proof = %+v, want proven %v", proof, test.proven)
			}
		})
	}
}

func TestCallbackBindingsBudget(t *testing.T) {
	pkg := buildTestSSA(t, callbackBindingsFixture)
	fn := pkg.Func("forwarded")
	proof := ProveCompletion(CompletionRequest{
		Instruction: findLaunch(t, fn), Target: fn.Params[0],
		Methods: []string{"Close"}, Budget: NewSearchBudget(1),
	})
	if proof.State != EvidenceUnknown || proof.Reason != EvidenceBudgetExhausted {
		t.Fatalf("exhausted callback search = %+v", proof)
	}
}
