package ssaflow

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCapturedBindingValueBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "captures", `package captures
func subject() int { n:=1; n=2; f:=func() int { return n }; return f() }
`)
	closure := InstructionsOf[*ssa.MakeClosure](pkg.Func("subject"))[0]
	binding := closure.Bindings[0]
	budget := proofs.NewSearchBudget(0)
	if got := CapturedBindingValueWithin(binding, budget); got != nil || !budget.Exhausted() {
		t.Fatalf("cutoff returned %v, exhausted=%v", got, budget.Exhausted())
	}
	// This query retains the historical first-initializer candidate semantics;
	// it must not pretend the returned value is the stable value at invocation.
	got := CapturedBindingValueWithin(binding, proofs.NewSearchBudget(proofs.QueryBudget))
	literal, ok := got.(*ssa.Const)
	if !ok || literal.Int64() != 1 {
		t.Fatalf("fresh candidate = %v, want initial 1", got)
	}
	if CapturedBindingValue(binding) != got {
		t.Fatal("default facade must share possible-value selection")
	}
}
