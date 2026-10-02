package lifecyclefacts

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
		state   ssaflow.EvidenceState
		effects ssaflow.CallEffect
	}{
		{"observed", ssaflow.EvidenceProven, ssaflow.EffectRead},
		{"launched", ssaflow.EvidenceProven, ssaflow.EffectRead | ssaflow.EffectAsync},
		{"unreadable", ssaflow.EvidenceUnknown, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			baseline := evidence.CallEffectsWithin(call, fn.Params[0], nil)
			if baseline.State != test.state || baseline.Effects != test.effects {
				t.Fatalf("default effects = %+v, want state %v effects %v", baseline, test.state, test.effects)
			}
			for limit := 0; limit <= ssaflow.QueryBudget; limit++ {
				budget := ssaflow.NewSearchBudget(limit)
				got := evidence.CallEffectsWithin(call, fn.Params[0], budget)
				if budget.Exhausted() || limit == 0 {
					if got.State != ssaflow.EvidenceUnknown || got.Reason != ssaflow.EvidenceBudgetExhausted {
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
	for range ssaflow.QueryBudget + 100 {
		source.WriteString("read(p)\n")
	}
	source.WriteString("}\nfunc caller(p *resource) { many(p) }")
	pkg := ssaflowtest.BuildPackage(t, "effectchild", source.String())
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	evidence := NewLifecycleEvidence(nil, "test", "test/local-effects")
	budget := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	got := evidence.CallEffectsWithin(call, fn.Params[0], budget)
	if got.State != ssaflow.EvidenceUnknown || got.Reason != ssaflow.EvidenceBudgetExhausted || budget.Exhausted() {
		t.Fatalf("child cutoff with available caller = %+v, caller exhausted %v", got, budget.Exhausted())
	}
	if baseline := evidence.CallEffectsWithin(call, fn.Params[0], nil); baseline != got {
		t.Fatalf("default cap = %+v, want %+v", baseline, got)
	}
}
