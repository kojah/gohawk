package lifecyclefacts

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
