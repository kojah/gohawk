package lifecycle_test

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const summaryFixture = `package summaries
func leaf() {}
func left() { leaf() }
func right() { leaf() }
func diamond() { left(); right() }
func recursiveA() { recursiveB() }
func recursiveB() { recursiveA(); leaf() }
func opaque()
func effect(ch chan int) { close(ch) }
func calls(a, b chan int) { effect(a); effect(b) }
func closure(ch chan int) { func() { close(ch) }() }
func dynamic(fn func()) { fn() }
`

// These counts are symbolic, additive summaries. Tests deliberately use a
// different summary shape from the channel protocol's ordered event sequence.
type countedSummary struct {
	count  int
	known  bool
	reason ssacall.SummaryUnavailable
}

func countSummaries(visits map[*ssa.Function]int) *ssacall.FunctionSummaries[countedSummary] {
	var summaries *ssacall.FunctionSummaries[countedSummary]
	summaries = ssacall.NewFunctionSummaries(func(function *ssa.Function, budget *proofs.SearchBudget) countedSummary {
		visits[function]++
		result := countedSummary{known: true, count: 1}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if !budget.Spend() {
					return result // Infrastructure must discard this partial answer.
				}
				if call, ok := instruction.(*ssa.Call); ok {
					nested := summaries.Function(call.Common().StaticCallee(), budget)
					if !nested.known {
						return nested
					}
					result.count += nested.count
				}
			}
		}
		return result
	}, func(reason ssacall.SummaryUnavailable) countedSummary {
		return countedSummary{reason: reason}
	})
	return summaries
}

func TestFunctionSummariesReuseAndIsolation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	visits := map[*ssa.Function]int{}
	summaries := countSummaries(visits)
	root := pkg.Func("diamond")
	if got := summaries.Function(root, proofs.NewSearchBudget(100)); !got.known || got.count != 5 {
		t.Fatalf("diamond summary = %+v, want known count 5", got)
	}
	for _, name := range []string{"diamond", "left", "right", "leaf"} {
		if visits[pkg.Func(name)] != 1 {
			t.Errorf("%s computed %d times, want 1", name, visits[pkg.Func(name)])
		}
	}
	if got := summaries.Function(root, proofs.NewSearchBudget(0)); !got.known || got.count != 5 {
		t.Errorf("cached answer requires no new traversal: %+v", got)
	}
	other := countSummaries(map[*ssa.Function]int{})
	if got := other.Function(root, proofs.NewSearchBudget(0)); got.known || got.reason != ssacall.SummaryBudgetExhausted {
		t.Errorf("separate engine reused another policy's answer: %+v", got)
	}
}

func TestFunctionSummariesUnavailable(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	for _, test := range []struct {
		name   string
		limit  int
		reason ssacall.SummaryUnavailable
	}{
		{"missing", 100, ssacall.SummaryBodyUnavailable},
		{"opaque", 100, ssacall.SummaryBodyUnavailable},
		{"recursiveA", 100, ssacall.SummaryRecursive},
		{"recursiveB", 100, ssacall.SummaryRecursive},
		{"diamond", 2, ssacall.SummaryBudgetExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			visits := map[*ssa.Function]int{}
			summaries := countSummaries(visits)
			function := pkg.Func(test.name)
			for range 2 {
				if got := summaries.Function(function, proofs.NewSearchBudget(test.limit)); got.known || got.reason != test.reason || got.count != 0 {
					t.Errorf("summary = %+v, want unavailable reason %v with no partial count", got, test.reason)
				}
			}
			if function != nil && len(function.Blocks) > 0 && visits[function] != 2 {
				t.Errorf("shortened answer cached: computations = %d, want 2", visits[function])
			}
			if test.name == "diamond" {
				if got := summaries.Function(function, proofs.NewSearchBudget(100)); !got.known || got.count != 5 {
					t.Errorf("fresh budget failed to recover: %+v", got)
				}
			}
		})
	}
}

func TestFunctionSummariesCallBindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	summaries := ssacall.NewFunctionSummaries(func(function *ssa.Function, _ *proofs.SearchBudget) []ssa.Value {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if call, ok := instruction.(*ssa.Call); ok {
					return []ssa.Value{call.Common().Args[0]}
				}
			}
		}
		return nil
	}, func(ssacall.SummaryUnavailable) []ssa.Value { return nil })
	caller := pkg.Func("calls")
	for index, call := range ssaflow.InstructionsOf[*ssa.Call](caller) {
		got := summaries.AtCall(call, proofs.NewSearchBudget(10), func(symbolic []ssa.Value, bindings []ssacall.CallBinding) []ssa.Value {
			if len(symbolic) != 1 || len(bindings) != 1 || symbolic[0] != bindings[0].Local || bindings[0].Captured {
				t.Fatalf("unexpected direct bindings: %+v for %v", bindings, symbolic)
			}
			return []ssa.Value{bindings[0].Supplied}
		})
		if len(got) != 1 || got[0] != caller.Params[index] {
			t.Errorf("call %d binding = %v, want parameter %v", index, got, caller.Params[index])
		}
	}
	for _, name := range []string{"closure", "dynamic"} {
		call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name))[0]
		bound := false
		summaries.AtCall(call, proofs.NewSearchBudget(10), func(_ []ssa.Value, bindings []ssacall.CallBinding) []ssa.Value {
			bound = true
			if len(bindings) != 1 || !bindings[0].Captured {
				t.Errorf("closure bindings = %+v", bindings)
			}
			return nil
		})
		if bound != (name == "closure") {
			t.Errorf("%s binding called = %v", name, bound)
		}
	}
}

func TestFunctionSummariesBindingBudgetDoesNotPoisonCallee(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	summaries := countSummaries(map[*ssa.Function]int{})
	callee := pkg.Func("leaf")
	if got := summaries.Function(callee, proofs.NewSearchBudget(10)); !got.known {
		t.Fatal("failed to warm callee summary")
	}
	call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("left"))[0]
	budget := proofs.NewSearchBudget(0)
	got := summaries.AtCall(call, budget, func(answer countedSummary, _ []ssacall.CallBinding) countedSummary {
		budget.Spend()
		return answer // Even a successful answer cannot escape an exhausted binding.
	})
	if got.known || got.reason != ssacall.SummaryBudgetExhausted || got.count != 0 {
		t.Errorf("partial bound answer escaped: %+v", got)
	}
	if cached := summaries.Function(callee, proofs.NewSearchBudget(0)); !cached.known || cached.count != 1 {
		t.Errorf("call-local binding failure poisoned symbolic callee: %+v", cached)
	}
}

func TestFunctionSummariesAlreadyExhaustedBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	summaries := countSummaries(map[*ssa.Function]int{})
	function := pkg.Func("leaf")
	if got := summaries.Function(function, nil); !got.known {
		t.Fatal("unbounded query failed")
	}
	budget := proofs.NewSearchBudget(0)
	budget.Spend()
	if got := summaries.Function(function, budget); got.known || got.reason != ssacall.SummaryBudgetExhausted {
		t.Errorf("cached evidence escaped an already exhausted query: %+v", got)
	}
	if got := summaries.Function(function, proofs.NewSearchBudget(0)); !got.known {
		t.Errorf("fresh query lost the completed summary: %+v", got)
	}
}
