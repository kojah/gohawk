package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
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
			if fact.InvokedParameters().contains(0) != test.want || fact.SynchronouslyInvoked.contains(0) != test.want {
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
		Analyzer: Summaries{call.Common().StaticCallee(): {Discharges: []Discharge{{Parameter: 0, Method: InvokeMethod}}}},
	}
	proof := NewLifecycleEvidence(pass, "test", "test/check").Prove(EvidenceRequest{
		Instruction: call, Target: fn.Params[0], SelectMask: func(fact Fact) ParameterMask { return fact.InvokedParameters() },
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

// A completion search that runs out of budget has decided nothing, so a
// transfer check that finds no handoff must not turn it into a disproof.
func TestAbandonedCompletionStaysUndecided(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
type command struct{ done bool }
func (c *command) Wait() { c.done = true }
func helper(c *command, n int) {
	for i := 0; i < n; i++ {
		if i%2 == 0 { continue }
	}
	c.Wait()
}
func run(c *command, n int) { helper(c, n) }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	fn := pkg.Func("run")
	call := findLifecycleCall(t, fn, "helper")
	target := fn.Params[0]
	proof := NewLifecycleEvidence(pass, "test", "test/check").Prove(EvidenceRequest{
		Instruction: call, Target: target,
		Completion: &lifecycle.CompletionRequest{
			Instruction: call, Target: target, Methods: []string{"Wait"}, Budget: ssaflow.NewSearchBudget(1),
		},
		Transfer: &lifecycle.OwnershipTransferRequest{Instruction: call, Value: target, Modes: lifecycle.TransferStoredInGlobal},
	})
	if proof.State == ssaflow.EvidenceDisproven || proof.Reason != ssaflow.EvidenceBudgetExhausted {
		t.Errorf("abandoned completion = %#v, want an undecided budget-exhausted proof", proof)
	}
}
