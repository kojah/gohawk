package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

const conditionalCompletionFixture = `package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func maybe(r *resource, yes bool) bool { if yes { r.Close(); return true }; return false }
func forward(r *resource, yes bool) bool { return maybe(r, yes) }
func wrong(r, other *resource, yes bool) bool { return maybe(other, yes) }
func lie(r *resource, yes bool) bool { if yes { return true }; r.Close(); return false }
func partial(r *resource, yes, skip bool) bool { if yes { if !skip { r.Close() }; return true }; return false }
func async(r *resource, yes bool) bool { if yes { go r.Close(); return true }; return false }
func deferred(r *resource, yes bool) bool { if yes { defer r.Close(); return true }; return false }
func noMatch(r *resource) bool { r.Close(); return false }
func nilSuccess(r *resource, yes bool, err error) error { if yes { r.Close(); return nil }; return err }
func recurse(r *resource, yes bool) bool { return recurse(r, yes) }
func invoke(fn func(), yes bool) bool { if yes { fn(); return true }; return false }
func good(r *resource, yes bool) { if maybe(r, yes) { return }; r.Close() }
func forwarded(r *resource, yes bool) { if forward(r, yes) { return }; r.Close() }
func other(r, another *resource, yes bool) { if wrong(r, another, yes) { return }; r.Close() }
func falseResult(r *resource, yes bool) { if !lie(r, yes) { return }; r.Close() }
func lying(r *resource, yes bool) { if lie(r, yes) { return }; r.Close() }
func partialResult(r *resource, yes, skip bool) { if partial(r, yes, skip) { return }; r.Close() }
func launched(r *resource, yes bool) { if async(r, yes) { return }; r.Close() }
func deferResult(r *resource, yes bool) { if deferred(r, yes) { return }; r.Close() }
func impossible(r *resource) { if noMatch(r) { return }; r.Close() }
func errorResult(r *resource, yes bool, err error) { if nilSuccess(r, yes, err) == nil { return }; r.Close() }
func recursive(r *resource, yes bool) { if recurse(r, yes) { return }; r.Close() }
func cancel(fn func(), yes bool) { if invoke(fn, yes) { return }; fn() }
type failure struct{}
func (failure) Error() string { return "failed" }
func tryClose(r *resource, yes bool) error { if !yes { return failure{} }; r.Close(); return nil }
func tryForward(r *resource, yes bool) error { return tryClose(r, yes) }
func errorSuccess(r *resource, yes bool) { if tryForward(r, yes) == nil { return }; r.Close() }
func tuple(r *resource, yes bool) (int, bool) { return 1, maybe(r, yes) }
func tupleResult(r *resource, yes bool) { _, ok := tuple(r, yes); if ok { return }; r.Close() }
func panicOnly(r *resource) bool { panic("not completion") }
func panicking(r *resource) { if panicOnly(r) { return }; r.Close() }
func closeOnError(r *resource, yes bool) error { if !yes { return nil }; r.Close(); return failure{} }
func errorFailure(r *resource, yes bool) { if closeOnError(r, yes) != nil { return }; r.Close() }
type pointerError struct{}
func (*pointerError) Error() string { return "typed nil" }
func typedNil(r *resource, yes bool) error { if yes { r.Close(); return nil }; var err *pointerError; return err }
func typedNilFailure(r *resource, yes bool) { if typedNil(r, yes) != nil { return }; r.Close() }
func typedNilSuccess(r *resource, yes bool) { if typedNil(r, yes) == nil { return }; r.Close() }
`

func TestConditionalCompletion(t *testing.T) {
	pkg := buildTestSSA(t, conditionalCompletionFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"good", true},
		{"forwarded", true},
		{"other", false},
		{"falseResult", true},
		{"lying", false},
		{"partialResult", false},
		{"launched", false},
		{"deferResult", true},
		{"impossible", false},
		{"errorResult", false},
		{"recursive", false},
		{"cancel", true},
		{"errorSuccess", true},
		{"tupleResult", true},
		{"panicking", false},
		{"errorFailure", true},
		{"typedNilFailure", false},
		{"typedNilSuccess", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			branch := InstructionsOf[*ssa.If](fn)[0].Block()
			request := CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}, Budget: NewSearchBudget(1000)}
			if test.name == "cancel" {
				request.Methods, request.InvokeTarget = nil, true
			}
			successor := branch.Succs[0]
			if test.name == "falseResult" {
				successor = branch.Succs[1]
			}
			proof := ProveCompletionOnEdge(branch, successor, request)
			if proof.Proven() != test.want {
				t.Fatalf("proof = %+v, want proven %v", proof, test.want)
			}
		})
	}
}

func TestConditionalCompletionMemoIsolation(t *testing.T) {
	pkg := buildTestSSA(t, conditionalCompletionFixture)
	fn := pkg.Func("good")
	call := InstructionsOf[*ssa.Call](fn)[0]
	search := newCompletionSearch("Close", CoverageEveryReturn, NewSearchBudget(1000))
	search.exactTarget = true
	for _, test := range []struct {
		kind completionConditionKind
		want bool
	}{
		{completionTrue, true},
		{completionFalse, false},
		{completionUnconditional, false},
		{completionTrue, true},
	} {
		search.condition = completionCondition{kind: test.kind}
		if _, proven, _ := search.completes(call, fn.Params[0]); proven != test.want {
			t.Errorf("condition %v: proven %v, want %v", test.kind, proven, test.want)
		}
	}
}

func TestConditionalCompletionDoesNotBecomeUnconditional(t *testing.T) {
	pkg := buildTestSSA(t, conditionalCompletionFixture)
	fn := pkg.Func("good")
	branch := InstructionsOf[*ssa.If](fn)[0].Block()
	request := CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}, Budget: NewSearchBudget(1000)}
	if proof := ProveCompletionOnEdge(branch, branch.Succs[0], request); !proof.Proven() {
		t.Fatalf("true edge: %+v", proof)
	}
	if proof := ProveCompletionOnEdge(branch, branch.Succs[1], request); proof.Proven() {
		t.Fatalf("false edge: %+v", proof)
	}
	request.Instruction = InstructionsOf[*ssa.Call](fn)[0]
	if proof := ProveCompletion(request); proof.Proven() {
		t.Fatalf("unconditional: %+v", proof)
	}
	request.Budget = NewSearchBudget(1)
	if proof := ProveCompletionOnEdge(branch, branch.Succs[0], request); proof.State != EvidenceUnknown {
		t.Fatalf("exhausted query: %+v", proof)
	}
}
