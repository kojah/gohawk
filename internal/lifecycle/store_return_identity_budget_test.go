package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestReturnedParameterSharedAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "identity", `package identity
type wrapper struct { p *int }
func direct(p *int) *int { return p }
func branches(p *int, flag bool) *int { if flag { return p }; return p }
func mixed(p *int, flag bool) *int { q := p; if flag { q = new(int) }; return q }
func boxed(p *int) any { return p }
func wrapped(p *int) wrapper { return wrapper{p} }
func forever(p *int) *int { for {} }
func recovered(p *int) *int { defer func() { recover() }(); return p }
`)
	for _, test := range []struct {
		name string
		want bool
	}{{"direct", true}, {"branches", true}, {"mixed", false}, {"boxed", false}, {"wrapped", false}, {"forever", false}, {"recovered", true}} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			t.Log(function.String())
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			fresh := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
			proof := ProveReturnedParameterWithin(function, function.Params[0], 0, fresh)
			if proof.Proven() != test.want || fresh.Exhausted() {
				t.Fatalf("fresh proof = %+v, want proven %v", proof, test.want)
			}
			if ReturnsParameterUnchanged(function, function.Params[0], 0) != test.want {
				t.Fatal("default facade disagrees")
			}
			if !test.want {
				return
			}
			completed := false
			for limit := 0; limit <= ssaflow.QueryBudget; limit++ {
				cut := ssaflow.NewSearchBudget(limit)
				got := ProveReturnedParameterWithin(function, function.Params[0], 0, cut)
				if got.Proven() {
					completed = true
					break
				}
				if got.State != ssaflow.EvidenceUnknown || got.Reason != ssaflow.EvidenceBudgetExhausted || !cut.Exhausted() {
					t.Fatalf("allowance %d admitted incomplete proof: %+v", limit, got)
				}
			}
			if !completed {
				t.Fatal("no complete allowance found")
			}
			pool := ssaflow.NewSearchBudget(0)
			got := ProveReturnedParameterWithin(function, function.Params[0], 0, pool.Within(ssaflow.QueryBudget))
			if got.Proven() || got.Reason != ssaflow.EvidenceBudgetExhausted || !pool.Exhausted() {
				t.Fatal("exhausted pool admitted identity")
			}
		})
	}
}
