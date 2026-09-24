package ssainfer_test

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// These contracts cover composition boundaries, not analyzer policy. In
// particular a recursive cut may preserve independent positive witnesses;
// it must not make that path-dependent answer reusable as a complete summary.
func TestSummaryContractNestedCutsInvalidateParents(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", `package contracts
func root() { child() }
func child() { root() }
func sibling() {}
`)
	for _, cut := range []string{"recursion", "budget"} {
		t.Run(cut, func(t *testing.T) {
			memo := ssaflow.NewCallGraphMemo[*ssa.Function, int]()
			root, child, sibling := pkg.Func("root"), pkg.Func("child"), pkg.Func("sibling")
			budget := ssaflow.NewSearchBudget(0)
			if cut == "recursion" {
				budget = ssaflow.NewSearchBudget(10)
			}
			unavailable := func(ssaflow.SummaryUnavailable, int) int { return -1 }
			got := memo.Summarize(root, root, budget, func() int {
				memo.Summarize(sibling, sibling, budget, func() int { return 7 }, unavailable)
				return memo.Summarize(child, child, budget, func() int {
					if cut == "budget" {
						budget.Spend()
						return 1
					}
					memo.Summarize(root, root, budget, func() int {
						t.Error("recursive body executed")
						return 99
					}, unavailable)
					return 1 // An independent witness survives, but is not cacheable.
				}, unavailable)
			}, unavailable)
			if (cut == "budget" && got != -1) || (cut == "recursion" && got != 1) {
				t.Fatalf("cut result = %d", got)
			}
			for _, function := range []*ssa.Function{root, child} {
				got := memo.Summarize(function, function, ssaflow.NewSearchBudget(10), func() int { return 42 }, unavailable)
				if got != 42 {
					t.Errorf("%s retained shortened answer %d", function.Name(), got)
				}
			}
			got = memo.Summarize(sibling, sibling, ssaflow.NewSearchBudget(0), func() int {
				t.Error("unaffected sibling was unnecessarily recomputed")
				return 99
			}, unavailable)
			if got != 7 {
				t.Errorf("independent completed sibling = %d, want 7", got)
			}
		})
	}
}

func TestSummaryContractBindingOwnsItsResult(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", `package contracts
func helper(ch chan int) { close(ch) }
func caller(a, b chan int) { helper(a); helper(b) }
`)
	callee, caller := pkg.Func("helper"), pkg.Func("caller")
	computations := 0
	summaries := ssaflow.NewFunctionSummaries(func(fn *ssa.Function, budget *ssaflow.SearchBudget) []ssa.Value {
		computations++
		budget.Spend()
		return []ssa.Value{fn.Params[0]}
	}, func(ssaflow.SummaryUnavailable) []ssa.Value { return nil })
	for index, call := range ssaflow.InstructionsOf[*ssa.Call](caller) {
		got := summaries.AtCall(call, ssaflow.NewSearchBudget(10), func(symbolic []ssa.Value, bindings []ssaflow.CallBinding) []ssa.Value {
			// Generic summary values can contain arbitrary maps and pointers.
			// Ownership is the adapter's contract, not an implicit deep copy.
			bound := slices.Clone(symbolic)
			bound[0] = bindings[0].Supplied
			return bound
		})
		if len(got) != 1 || got[0] != caller.Params[index] {
			t.Fatalf("invocation %d bound to %v", index, got)
		}
		got[0] = nil
		if symbolic := summaries.Function(callee, ssaflow.NewSearchBudget(0)); symbolic[0] != callee.Params[0] {
			t.Fatal("call-site mutation changed cached symbolic evidence")
		}
	}
	if computations != 1 {
		t.Errorf("callee computed %d times, want 1", computations)
	}
}

func TestSummaryContractPolicyIsolation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", "package contracts; func helper() {}")
	for _, policyAnswer := range []int{7, 11} {
		summaries := ssaflow.NewFunctionSummaries(func(_ *ssa.Function, budget *ssaflow.SearchBudget) int {
			budget.Spend()
			return policyAnswer
		}, func(ssaflow.SummaryUnavailable) int { return -1 })
		for _, limit := range []int{1, 0} {
			if got := summaries.Function(pkg.Func("helper"), ssaflow.NewSearchBudget(limit)); got != policyAnswer {
				t.Errorf("policy %d got %d", policyAnswer, got)
			}
		}
	}
}

func TestSummaryContractBindingCutInvalidatesCaller(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", `package contracts
func helper() {}
func caller() { helper() }
`)
	caller, callee := pkg.Func("caller"), pkg.Func("helper")
	call := ssaflow.InstructionsOf[*ssa.Call](caller)[0]
	visits := map[*ssa.Function]int{}
	var summaries *ssaflow.FunctionSummaries[int]
	summaries = ssaflow.NewFunctionSummaries(func(fn *ssa.Function, budget *ssaflow.SearchBudget) int {
		visits[fn]++
		budget.Spend()
		if fn == callee {
			return 7
		}
		return summaries.AtCall(call, budget, func(answer int, _ []ssaflow.CallBinding) int {
			budget.Spend()
			return answer
		})
	}, func(ssaflow.SummaryUnavailable) int { return -1 })
	if got := summaries.Function(caller, ssaflow.NewSearchBudget(2)); got != -1 {
		t.Fatalf("binding-exhausted caller = %d, want unavailable", got)
	}
	if got := summaries.Function(caller, ssaflow.NewSearchBudget(10)); got != 7 {
		t.Errorf("fresh caller query = %d, want 7", got)
	}
	if visits[caller] != 2 || visits[callee] != 1 {
		t.Errorf("computations caller=%d callee=%d, want 2 and 1", visits[caller], visits[callee])
	}
}
