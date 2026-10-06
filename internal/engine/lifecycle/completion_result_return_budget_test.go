package lifecycle

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"golang.org/x/tools/go/ssa"
)

const resultReturnBudgetFixture = resultGuardFixture + `
func twoResults(fail bool)(err error,ok bool){
 f,_:=open()
 defer func(){if err!=nil && ok{f.Close()}}()
 if fail{return &failure{},true};return nil,false
}
func earlierResult(fail bool)(err error){
 f,_:=open();defer func(){if err!=nil{f.Close()}}()
 if fail{err=&failure{}};return
}
func overwrittenResult()(err error){
 f,_:=open();defer func(){if err!=nil{f.Close()}}()
 err=&failure{};return nil
}
func conditionalRegistration(yes bool)(err error){
 f,_:=open();if yes{defer func(){if err!=nil{f.Close()}}()};return nil
}
`

func TestResultReturnBindingAllowance(t *testing.T) {
	pkg := buildTestSSA(t, resultReturnBudgetFixture)
	for _, name := range []string{"closeOnError", "twoResults", "earlierResult", "overwrittenResult"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}
			guards := returnBindingGuards(fn, request)
			if len(guards) != 1 {
				t.Fatalf("guards=%+v", guards)
			}
			guard := guards[0]
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			checked := 0
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
				if !cfg.InstructionDominates(guard.Defer, returned) {
					continue
				}
				checked++
				baseline := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome)
				if name == "earlierResult" && baseline != proofs.EvidenceUnknown {
					t.Fatalf("earlier result=%v", baseline)
				}
				if name == "overwrittenResult" && baseline != proofs.EvidenceDisproven {
					t.Fatalf("overwritten result=%v", baseline)
				}
				checkReturnStoresWithin(t, returned, guard.Cells)
				checkReturnCompletionWithin(t, guard, request, returned, baseline)
				request.Budget = nil
			}
			if checked == 0 {
				t.Fatal("no guarded return")
			}
		})
	}
}

func TestResultReturnCallbackChildAndFresh(t *testing.T) {
	fn := buildTestSSA(t, resultReturnBudgetFixture).Func("twoResults")
	request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}
	guards := returnBindingGuards(fn, request)
	if len(guards) != 1 {
		t.Fatalf("guards=%+v", guards)
	}
	guard := guards[0]
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
		if !cfg.InstructionDominates(guard.Defer, returned) {
			continue
		}
		pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
		request.Budget = pool.Within(1)
		if got := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome); got != proofs.EvidenceUnknown || pool.Exhausted() {
			t.Fatalf("child=%v pool exhausted=%v", got, pool.Exhausted())
		}
		request.Budget = pool.Within(proofs.SummaryBudget)
		baseline := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome)
		if baseline == proofs.EvidenceUnknown {
			t.Fatal("fresh binding remained unknown")
		}
		request.Budget = pool.Within(proofs.SummaryBudget)
		outcome := func(value ssa.Value) (ssacall.Outcome, bool) {
			for request.Budget.Spend() {
			}
			return ssacall.ValueOutcome(value)
		}
		if got := guard.CompletesAtReturn(request, returned, outcome); got != proofs.EvidenceUnknown || pool.Exhausted() {
			t.Fatalf("callback cut=%v pool exhausted=%v", got, pool.Exhausted())
		}
		request.Budget = pool.Within(proofs.SummaryBudget)
		if got := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome); got != baseline {
			t.Fatalf("fresh=%v want%v", got, baseline)
		}
	}
}

func TestResultGuardReturnReachabilityAllowance(t *testing.T) {
	pkg := buildTestSSA(t, resultReturnBudgetFixture)
	for _, name := range []string{"closeOnError", "conditionalRegistration"} {
		fn := pkg.Func(name)
		guards := ProveResultGuards(fn, CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}).Guards
		if len(guards) != 1 {
			t.Fatalf("%s guards=%+v", name, guards)
		}
		guard := guards[0]
		for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
			want := proofs.EvidenceDisproven
			if cfg.InstructionDominates(guard.Defer, returned) {
				want = proofs.EvidenceProven
			} else if cfg.InstructionMayFollow(guard.Defer, returned) {
				want = proofs.EvidenceUnknown
			}
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := guard.ProveReachesReturn(returned, budget)
				if budget.Exhausted() || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				if got.State != want {
					t.Fatalf("reach=%+v want%v", got, want)
				}
				break
			}
		}
	}
}

// Discovery intentionally does not infer a conjunction by varying only one
// result. Construct the two-cell binding question directly, without expanding
// the discovery contract under test elsewhere.
func returnBindingGuards(fn *ssa.Function, request CompletionRequest) []ResultGuard {
	if fn.Name() != "twoResults" {
		return ProveResultGuards(fn, request).Guards
	}
	for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](fn) {
		closure, ok := deferred.Call.Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		guard := ResultGuard{Defer: deferred}
		named := ssaflow.ProveNamedResultCellsWithin(fn, nil)
		for _, binding := range closure.Bindings {
			if cell, ok := binding.(*ssa.Alloc); ok {
				if _, found := named.Cells[cell]; found {
					guard.Cells = append(guard.Cells, cell)
				}
			}
		}
		return []ResultGuard{guard}
	}
	return nil
}

func checkReturnStoresWithin(t *testing.T, returned *ssa.Return, cells []*ssa.Alloc) {
	t.Helper()
	for _, cell := range cells {
		want, found := ssaflow.ValueAtReturnWithin(returned, cell, nil)
		completed := false
		for limit := 0; limit <= proofs.QueryBudget; limit++ {
			budget := proofs.NewSearchBudget(limit)
			got, ok := ssaflow.ValueAtReturnWithin(returned, cell, budget)
			if budget.Exhausted() || limit == 0 {
				if got != nil || ok {
					t.Fatalf("cut%d retained store %v", limit, got)
				}
				continue
			}
			if got != want || ok != found {
				t.Fatalf("store changed: %v/%v want %v/%v", got, ok, want, found)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("store lookup never completed")
		}
	}
}

func checkReturnCompletionWithin(t *testing.T, guard ResultGuard, request CompletionRequest, returned *ssa.Return, want proofs.EvidenceState) {
	t.Helper()
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		request.Budget = proofs.NewSearchBudget(limit)
		got := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome)
		if request.Budget.Exhausted() || limit == 0 {
			if got != proofs.EvidenceUnknown {
				t.Fatalf("cut%d completion=%v", limit, got)
			}
			continue
		}
		if got != want {
			t.Fatalf("complete=%v want%v", got, want)
		}
		return
	}
	t.Fatal("binding never completed")
}
