package lifecyclefacts

import (
	"go/types"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/analysis"
)

func TestCallbackInferenceUsesBoundedPackageSummaries(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
func invoke(fn func() error) { _ = fn() }
func forward(fn func() error) { invoke(fn) }
func outer(fn func() error) { forward(fn) }
func asynchronous(fn func() error) { go fn() }
func asyncForward(fn func() error) { asynchronous(fn) }
func conditional(fn func() error, yes bool) { if yes { invoke(fn) } }
func replacement(fn func() error) { invoke(func() error { return nil }) }
func recursive(fn func() error) { recursive(fn) }
func cyclicA(fn func() error) { cyclicB(fn) }
func cyclicB(fn func() error) { cyclicA(fn) }
`)
	pass := &analysis.Pass{Pkg: pkg.Pkg, ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	callbacks := newCallbackInference(pass, nil)
	for _, test := range []struct {
		name        string
		invoked     bool
		synchronous bool
	}{
		{"outer", true, true},
		{"asyncForward", true, false},
		{"conditional", false, false},
		{"replacement", false, false},
		{"recursive", false, false},
		{"cyclicA", false, false},
		{"cyclicB", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact := callbacks.invocations.Function(pkg.Func(test.name), proofs.NewSearchBudget(proofs.SummaryBudget))
			if fact.InvokedParameters().contains(0) != test.invoked || fact.SynchronouslyInvoked().contains(0) != test.synchronous {
				t.Fatalf("invocation = %v, synchronous = %v; want %v, %v",
					fact.InvokedParameters(), fact.SynchronouslyInvoked(), test.invoked, test.synchronous)
			}
		})
	}
}

func TestCallbackInferenceDoesNotCacheExhaustedClaims(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
func invoke(fn func() error) { _ = fn() }
func forward(fn func() error) { invoke(fn) }
`)
	pass := &analysis.Pass{Pkg: pkg.Pkg, ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	callbacks := newCallbackInference(pass, nil)
	short := callbacks.invocations.Function(pkg.Func("forward"), proofs.NewSearchBudget(1))
	if len(short.Discharges) != 0 {
		t.Fatalf("exhausted summary advertised discharges: %v", short.Discharges)
	}
	complete := callbacks.invocations.Function(pkg.Func("forward"), proofs.NewSearchBudget(proofs.SummaryBudget))
	if !complete.InvokedParameters().contains(0) || !complete.SynchronouslyInvoked().contains(0) {
		t.Fatalf("fresh budget reused an incomplete answer: %v", complete.Discharges)
	}
}
