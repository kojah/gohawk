package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
)

func TestInvocationSummaryRequiresDefiniteIdentity(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
func chosen(a, b func(), pick bool) {
	fn := a
	if pick { fn = b }
	fn()
}
func exact(a func()) { a() }
func helper(fn func()) {}
func forward(a, b func(), pick bool) {
	fn := a
	if pick { fn = b }
	helper(fn)
}
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for _, test := range []struct {
		name string
		want bool
	}{
		{"chosen", false}, {"exact", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact := summarize(pass, pkg.Func(test.name))
			if fact.Invoked.contains(0) != test.want || fact.SynchronouslyInvoked.contains(0) != test.want {
				t.Errorf("summary = %#v, want invocation of parameter 0: %t", fact, test.want)
			}
		})
	}
	fn := pkg.Func("forward")
	call := findLifecycleCall(t, fn, "helper")
	if factOwnsExactArgument(call, fn.Params[0], parameterMaskFor(0)) {
		t.Fatal("a summary for the selected argument must not prove action on either possible argument")
	}
	pass.ResultOf = map[*analysis.Analyzer]any{
		Analyzer: Summaries{call.Common().StaticCallee(): {Invoked: parameterMaskFor(0)}},
	}
	proof := NewLifecycleEvidence(pass, "test", "test/check").Prove(EvidenceRequest{
		Instruction: call, Target: fn.Params[0], SelectMask: func(fact Fact) ParameterMask { return fact.Invoked },
	})
	if proof.State != ssaflow.EvidenceUnknown {
		t.Errorf("ambiguous imported callback proof = %#v, want unknown", proof)
	}
}

func TestReturnedUnchangedRequiresDefiniteIdentity(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
func chosen(a, b *int, pick bool) *int {
	value := a
	if pick { value = b }
	return value
}
func exact(a *int) *int { return a }
func deferred(a *int) *int {
	defer func() { _ = *a }()
	return a
}
func replaced(a, b *int) *int {
	defer func() { _ = *a }()
	a = b
	return a
}
func namedMutated(a, b *int) (result *int) {
	defer func() { result = b }()
	return a
}
func escaped(a *int, mutate func(**int)) *int {
	mutate(&a)
	return a
}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"chosen", false},
		{"exact", true},
		{"deferred", true},
		{"replaced", false},
		{"namedMutated", false},
		{"escaped", false},
	} {
		fn := pkg.Func(test.name)
		if got := parameterReturnedUnchangedOnEveryReturn(fn, fn.Params[0]); got != test.want {
			t.Errorf("%s returned unchanged = %t, want %t", test.name, got, test.want)
		}
	}
}
