package lifecyclefacts

import (
	"go/types"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

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
