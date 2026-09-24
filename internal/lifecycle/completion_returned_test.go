package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

const returnedCleanupFixture = `package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func (*resource) CloseErr() error { return nil }
func cleanup(r *resource) func() { return func() { r.Close() } }
func forward(r *resource) func() { return cleanup(r) }
func bound(r *resource) func() { return r.Close }
func pair() (*resource, func()) { r := new(resource); return r, func() { r.Close() } }
func pairForward() (*resource, func()) { return pair() }
func wrongPair() (*resource, func()) { r, other := new(resource), new(resource); return r, func() { other.Close() } }
func partial(r *resource, yes bool) func() { if yes { return cleanup(r) }; return func() {} }
func changed(r *resource) func() { f := func() { r.Close() }; r = new(resource); return f }
func async(r *resource) func() { return func() { go r.Close() } }
func recursive(r *resource) func() { return recursive(r) }
func wrap(fn func()) func() { return func() { fn() } }
func wrapErr(fn func() error) func() { return func() { _ = fn() } }
func ignoreErr(_ func() error) func() { return func() {} }
func direct(r *resource) { defer cleanup(r)() }
func forwarded(r *resource) { defer forward(r)() }
func method(r *resource) { defer bound(r)() }
func siblings() { r, fn := pair(); fn(); _ = r }
func siblingsForward() { r, fn := pairForward(); fn(); _ = r }
func wrongSibling() { r, fn := wrongPair(); fn(); _ = r }
func distinct() { r, _ := pair(); _, fn := pair(); fn(); _ = r }
func conditional(r *resource, yes bool) { defer partial(r, yes)() }
func reassigned(r *resource) { defer changed(r)() }
func launched(r *resource) { defer async(r)() }
func recursing(r *resource) { defer recursive(r)() }
func cancellation(fn func()) { defer wrap(fn)() }
func boundError(r *resource) { defer wrapErr(r.CloseErr)() }
func ignoredBoundError(r *resource) { defer ignoreErr(r.CloseErr)() }
`

func TestReturnedCleanupCompletion(t *testing.T) {
	pkg := buildTestSSA(t, returnedCleanupFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"direct", true},
		{"forwarded", true},
		{"method", true},
		{"siblings", true},
		{"siblingsForward", true},
		{"wrongSibling", false},
		{"distinct", false},
		{"conditional", false},
		{"reassigned", false},
		{"launched", false},
		{"recursing", false},
		{"cancellation", true},
		{"boundError", true},
		{"ignoredBoundError", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var target ssa.Value
			if len(function.Params) != 0 {
				target = function.Params[0]
			} else {
				target = ssaflow.InstructionsOf[*ssa.Extract](function)[0]
			}
			var invocation ssa.Instruction
			if deferred := ssaflow.InstructionsOf[*ssa.Defer](function); len(deferred) != 0 {
				invocation = deferred[0]
			} else {
				calls := ssaflow.InstructionsOf[*ssa.Call](function)
				invocation = calls[len(calls)-1]
			}
			request := CompletionRequest{Instruction: invocation, Target: target, Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(2000)}
			switch test.name {
			case "cancellation":
				request.Methods, request.InvokeTarget = nil, true
			case "boundError", "ignoredBoundError":
				request.Methods = []string{"CloseErr"}
			}
			if proof := ProveCompletion(request); proof.Proven() != test.want {
				t.Fatalf("proof = %+v, want proven %v", proof, test.want)
			}
		})
	}
}
