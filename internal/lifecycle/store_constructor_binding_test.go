package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedConstructorCallBinding(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "constructorbinding", `package constructorbinding
type owner struct { value *int }
type builder struct {}
func (*builder) Build(p *int) *owner { return &owner{p} }
type factory interface { Build(*int) *owner }
func direct(p *int) *owner { return (&builder{}).Build(p) }
func dynamic(p *int, f factory) *owner { return f.Build(p) }
func boxed(p *int) *owner { var f factory = &builder{}; return f.Build(p) }
`)
	for _, name := range []string{"direct", "dynamic", "boxed"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			t.Log(call.String())
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			if name == "direct" {
				if call.Common().IsInvoke() || len(call.Common().Args) != 2 || call.Common().Args[1] != fn.Params[0] {
					t.Fatal("direct method fixture lacks separate receiver and resource operands")
				}
			} else if !call.Common().IsInvoke() || call.Common().StaticCallee() != nil {
				t.Fatal("interface fixture no longer has unresolved dispatch")
			}
			proof := ProveReturnedOwnershipWithin(returned, fn.Params[0], nil, proofs.NewSearchBudget(proofs.SummaryBudget))
			if proof.State == proofs.EvidenceUnknown || proof.Proven() != (name == "direct") {
				t.Fatalf("constructor binding = %+v", proof)
			}
			// A completed decline supplies no owner evidence, not a claim
			// that a dynamically dispatched constructor cannot retain its input.
			cutoff := ProveReturnedOwnershipWithin(returned, fn.Params[0], nil, proofs.NewSearchBudget(0))
			if cutoff.State != proofs.EvidenceUnknown || cutoff.Reason != proofs.EvidenceBudgetExhausted {
				t.Fatalf("interrupted constructor binding = %+v", cutoff)
			}
		})
	}
}
