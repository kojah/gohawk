package resultfacts

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAssumedResultCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", relationFixture)
	function := pkg.Func("Failed")
	var value ssa.Value
	for _, instruction := range function.Blocks[0].Instrs {
		t.Log(instruction.String())
		if returned, ok := instruction.(*ssa.Return); ok {
			value = returned.Results[0]
		}
	}
	assumed := ssacall.FixedValues{function.Params[0]: ssacall.OutcomeNil}
	engine := NewEngine()
	zero := proofs.NewSearchBudget(0)
	if got := engine.assumedValue(value, assumed, zero); got != Unknown || !zero.Exhausted() {
		t.Fatal("zero allowance must not decide an assumed literal")
	}
	pool := proofs.NewSearchBudget(0)
	if got := engine.assumedValue(value, assumed, pool.Within(proofs.QueryBudget)); got != Unknown || !pool.Exhausted() {
		t.Fatal("exhausted parent pool must not decide an assumed literal")
	}
	completed := false
	for limit := 0; limit <= proofs.QueryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got := engine.assumedValue(value, assumed, budget)
		if got == AlwaysFalse {
			if budget.Exhausted() {
				t.Fatal("interrupted assumption must not prove a literal")
			}
			completed = true
			break
		}
		if got != Unknown || !budget.Exhausted() {
			t.Fatalf("cutoff assumption = %v, exhausted %v", got, budget.Exhausted())
		}
	}
	if !completed {
		t.Fatal("fresh assumption did not complete")
	}
}

func TestConditionalSummaryCutoffRecovery(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", relationFixture)
	for _, name := range []string{"Failed", "FailedWithSideEffect", "OpenForwarded"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			want := NewEngine().Function(function, proofs.NewSearchBudget(proofs.SummaryBudget))
			if !want.Available || len(want.Cases()) == 0 {
				t.Fatal("expected actual conditional guarantee")
			}
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				engine := NewEngine()
				budget := proofs.NewSearchBudget(limit)
				got := engine.Function(function, budget)
				if got.Available {
					if len(got.Cases()) != len(want.Cases()) || budget.Exhausted() {
						t.Fatal("complete summary changed its cases")
					}
					completed = true
					break
				}
				if len(got.Cases()) != 0 || !budget.Exhausted() {
					t.Fatal("interrupted summary admitted cases")
				}
				fresh := engine.Function(function, proofs.NewSearchBudget(proofs.SummaryBudget))
				if !fresh.Available || len(fresh.Cases()) != len(want.Cases()) {
					t.Fatal("cutoff poisoned a fresh query")
				}
			}
			if !completed {
				t.Fatal("conditional inference never completed")
			}
		})
	}
}
