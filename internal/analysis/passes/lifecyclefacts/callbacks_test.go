package lifecyclefacts

import (
	"go/types"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
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

// Capture selection is shared, but visible retention and unreadable handoff
// are different questions. A visible store may retain without an opaque call.
func TestClosureCaptureEvidenceBoundaries(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
var saved *int
func opaque(*int)
func read(p *int) { _ = *p }
func forward(p *int) *int { return p }
func retained(p,q *int) func() { return func() { saved=p } }
func unreadable(p,q *int) func() { return func() { opaque(p) } }
func visible(p,q *int) func() { return func() { read(p) } }
func unrelated(p,q *int) func() { return func() { opaque(q) } }
func derived(p,q *int) func() { v:=forward(p); return func() { opaque(v) } }
func multiple(p,q *int) func() { return func() { read(q); opaque(p) } }
`)
	pass := &analysis.Pass{
		Pkg:              pkg.Pkg,
		ResultOf:         map[*analysis.Analyzer]any{Analyzer: Summaries{}},
		ImportObjectFact: func(types.Object, analysis.Fact) bool { return false },
	}
	for _, test := range []struct {
		name                 string
		retained, unreadable bool
	}{
		{"retained", true, false},
		{"unreadable", true, true},
		{"visible", false, false},
		{"unrelated", false, false},
		{"derived", true, true},
		{"multiple", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			closures := ssaflow.InstructionsOf[*ssa.MakeClosure](function)
			if len(closures) != 1 {
				t.Fatal("expected one closure")
			}
			evidence := NewLifecycleEvidence(pass, "test", "test/check")
			if got := evidence.ClosureRetainsValue(closures[0], function.Params[0]); got != test.retained {
				t.Errorf("ClosureRetainsValue() = %t, want %t", got, test.retained)
			}
			if got := evidence.ClosureHandsValueToUnreadableCallee(closures[0], function.Params[0]); got != test.unreadable {
				t.Errorf("ClosureHandsValueToUnreadableCallee() = %t, want %t", got, test.unreadable)
			}
			cutoff := proofs.NewSearchBudget(0)
			if !evidence.ClosureHandsValueToUnreadableCalleeWithin(closures[0], function.Params[0], cutoff) || !cutoff.Exhausted() {
				t.Fatal("capture cutoff must preserve possible opaque consumption")
			}
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			if got := evidence.ClosureHandsValueToUnreadableCalleeWithin(closures[0], function.Params[0], fresh); got != test.unreadable || fresh.Exhausted() {
				t.Fatalf("fresh handoff=%v exhausted=%v, want %v", got, fresh.Exhausted(), test.unreadable)
			}
		})
	}
}

func TestLocalCallEffectsAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "effectallowance", `package effectallowance
type resource struct { n int }
func read(p *resource) { _ = p.n }
func async(p *resource) { go read(p) }
func opaque(*resource)
func observed(p *resource) { read(p) }
func launched(p *resource) { async(p) }
func unreadable(p *resource) { opaque(p) }
`)
	evidence := NewLifecycleEvidence(nil, "test", "test/local-effects")
	for _, test := range []struct {
		name    string
		state   proofs.EvidenceState
		effects ssacall.CallEffect
	}{
		{"observed", proofs.EvidenceProven, ssacall.EffectRead},
		{"launched", proofs.EvidenceProven, ssacall.EffectRead | ssacall.EffectAsync},
		{"unreadable", proofs.EvidenceUnknown, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			baseline := evidence.CallEffectsWithin(call, fn.Params[0], nil)
			if baseline.State != test.state || baseline.Effects != test.effects {
				t.Fatalf("default effects = %+v, want state %v effects %v", baseline, test.state, test.effects)
			}
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := evidence.CallEffectsWithin(call, fn.Params[0], budget)
				if budget.Exhausted() || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted effects: %+v", limit, got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete effects = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("effects never completed")
		})
	}
}

func TestLocalCallEffectsChildCap(t *testing.T) {
	var source strings.Builder
	source.WriteString(`package effectchild
type resource struct { n int }
func read(p *resource) { _ = p.n }
func many(p *resource) {
`)
	for range proofs.QueryBudget + 100 {
		source.WriteString("read(p)\n")
	}
	source.WriteString("}\nfunc caller(p *resource) { many(p) }")
	pkg := ssaflowtest.BuildPackage(t, "effectchild", source.String())
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	evidence := NewLifecycleEvidence(nil, "test", "test/local-effects")
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	got := evidence.CallEffectsWithin(call, fn.Params[0], budget)
	if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || budget.Exhausted() {
		t.Fatalf("child cutoff with available caller = %+v, caller exhausted %v", got, budget.Exhausted())
	}
	if baseline := evidence.CallEffectsWithin(call, fn.Params[0], nil); baseline != got {
		t.Fatalf("default cap = %+v, want %+v", baseline, got)
	}
}
