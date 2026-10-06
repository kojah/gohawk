package resultfacts

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const resultFoldFixture = `package results
type failure struct{}
func (*failure) Error() string { return "failure" }
type cell struct { value any }
func literal() bool { return true }
func boxed() any { var err error = (*failure)(nil); return err }
func agreeing(flag bool) error {
 var err error
 if flag { err = (*failure)(nil) } else { err = &failure{} }
 return err
}
func disagreeing(flag bool) error {
 var err error
 if flag { err = (*failure)(nil) }
 return err
}
func stored() any { var err error = (*failure)(nil); c := &cell{value: err}; return c.value }
`

func TestResultFoldSharedAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", resultFoldFixture)
	for _, test := range []struct {
		name string
		want Guarantee
	}{{"literal", AlwaysTrue}, {"boxed", AlwaysNonNil}, {"agreeing", AlwaysNonNil}, {"stored", AlwaysNonNil}, {"disagreeing", Unknown}} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var value ssa.Value
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
					if returned, ok := instruction.(*ssa.Return); ok {
						value = returned.Results[0]
					}
				}
			}
			engine := NewEngine()
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			if got := engine.value(value, fresh); got != test.want || fresh.Exhausted() {
				t.Fatalf("fresh value = %v, want %v", got, test.want)
			}
			if test.want == Unknown {
				return
			}
			checkResultFoldCutoffs(t, engine, value, test.want)
		})
	}
}

func checkResultFoldCutoffs(t *testing.T, engine *Engine, value ssa.Value, want Guarantee) {
	t.Helper()
	// A leaf allowance alone cannot cover its reaching-value visit.
	one := proofs.NewSearchBudget(1)
	if got := engine.value(value, one); got != Unknown || !one.Exhausted() {
		t.Fatal("leaf-only allowance admitted uncharged fold work")
	}
	completed := false
	for limit := 0; limit <= proofs.QueryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got := engine.value(value, budget)
		if got == want {
			if budget.Exhausted() {
				t.Fatal("exhaustion established a result guarantee")
			}
			completed = true
			break
		}
		if got != Unknown || !budget.Exhausted() {
			t.Fatalf("incomplete allowance %d = %v", limit, got)
		}
	}
	if !completed {
		t.Fatal("result fold never completed")
	}
	pool := proofs.NewSearchBudget(0)
	if got := engine.value(value, pool.Within(proofs.QueryBudget)); got != Unknown || !pool.Exhausted() {
		t.Fatal("pool cutoff admitted result evidence")
	}
}

func TestResultFoldSummaryRecovery(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", resultFoldFixture)
	for _, name := range []string{"boxed", "agreeing", "stored"} {
		t.Run(name, func(t *testing.T) {
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				engine := NewEngine()
				budget := proofs.NewSearchBudget(limit)
				got := engine.Function(pkg.Func(name), budget)
				if got.Available {
					if got.Result(0) != AlwaysNonNil || budget.Exhausted() {
						t.Fatal("completed result summary lost its guarantee")
					}
					completed = true
					break
				}
				if got.Result(0) != Unknown || !budget.Exhausted() {
					t.Fatal("incomplete result summary established a guarantee")
				}
				fresh := engine.Function(pkg.Func(name), proofs.NewSearchBudget(proofs.SummaryBudget))
				if !fresh.Available || fresh.Result(0) != AlwaysNonNil {
					t.Fatal("cutoff poisoned fresh result inference")
				}
			}
			if !completed {
				t.Fatal("result summary never completed")
			}
		})
	}
}
