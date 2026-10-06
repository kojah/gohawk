package calls_test

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const capturedTimingFixture = `package capturetiming
 func effect(){}
 func before(){done:=true;defer func(){if done{effect()}}()}
 func early(leave bool){var done bool;defer func(){if done{effect()}}();if leave{return};done=true}
 func preStore(){var done bool;func(){if done{effect()}}();done=true}
 func lateCall(){var done bool;f:=func(){if done{effect()}};done=true;f()}
 func branchBefore(leave bool){var done bool;if leave{return};done=true;defer func(){if done{effect()}}()}
 func conditional(choose bool){var done bool;if choose{done=true};defer func(){if done{effect()}}()}
 func lateNil(){var p *int;defer func(){if p==nil{effect()}}();p=new(int)}
 func named()(done bool){defer func(){if done{effect()}}();return true}
`

// Inferred bindings describe every read after capture. A unique write does
// not fix reads before it; a caller-supplied current cell is a separate proof.
func TestFixedCaptureTiming(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "capturetiming", capturedTimingFixture)
	for _, test := range []struct {
		name  string
		bound bool
		known bool
	}{
		{"before", true, false},
		{"early", false, false},
		{"preStore", false, false},
		{"lateCall", false, false},
		{"branchBefore", true, false},
		{"conditional", false, false},
		{"lateNil", false, false},
		{"named", false, false},
		{"named", true, true},
		{"lateCall", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := fixedBindingCall(t, fn)
			body, closure := ssacall.DirectCallee(ssaflow.InstructionCall(call))
			known := ssacall.FixedValues{}
			if test.known {
				known[closure.Bindings[0]] = ssacall.OutcomeTrue
			}
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			if _, err := body.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			proof := ssacall.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, body, known, nil)
			_, bound := proof.Values[body.FreeVars[0]]
			if !proof.Proven() || bound != test.bound {
				t.Fatalf("binding=%+v want bound=%v", proof, test.bound)
			}
			checkFixedBindingCutoffs(t, func(budget *proofs.SearchBudget) ssacall.FixedArgumentsProof {
				return ssacall.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, body, known, budget)
			}, proof)
		})
	}
}

func TestFixedCaptureOrderingCutoff(t *testing.T) {
	source := `package captureorder
 func effect(){}
 func padded(){done:=true;` + strings.Repeat("println();", proofs.QueryBudget+1) + `defer func(){if done{effect()}}()}
 `
	fn := ssaflowtest.BuildPackage(t, "captureorder", source).Func("padded")
	defers := ssaflow.InstructionsOf[*ssa.Defer](fn)
	if len(defers) != 1 {
		t.Fatal("expected one deferred closure")
	}
	call := defers[0]
	body, closure := ssacall.DirectCallee(ssaflow.InstructionCall(call))
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	proof := ssacall.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, body, nil, child)
	budgetUnknown := proof.State == proofs.EvidenceUnknown && proof.Reason == proofs.EvidenceBudgetExhausted
	if !budgetUnknown || proof.Values != nil || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("ordering cutoff=%+v child=%v parent=%v", proof, child.Exhausted(), pool.Exhausted())
	}
	fresh := ssacall.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, body, nil, pool.Within(2*proofs.SummaryBudget))
	if !fresh.Proven() || len(fresh.Values) != 1 || fresh.Values[body.FreeVars[0]] != ssacall.OutcomeTrue {
		t.Fatalf("fresh ordering=%+v", fresh)
	}
}
