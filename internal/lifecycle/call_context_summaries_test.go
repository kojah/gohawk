package lifecycle_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestSummaryContextKeys(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	function := pkg.Func("calls")
	type key struct {
		function *ssa.Function
		target   ssa.Value
		strict   bool
	}
	memo := ssaflow.NewCallGraphMemo[key, int]()
	for index, target := range function.Params {
		for _, strict := range []bool{false, true} {
			context := key{function: function, target: target, strict: strict}
			want := index + 1
			if strict {
				want += 10
			}
			for attempt := range 2 {
				got := memo.Summarize(context, function, nil, func() int {
					if attempt != 0 {
						t.Error("completed context was not memoized")
					}
					return want
				}, func(ssaflow.SummaryUnavailable, int) int { return -1 })
				if got != want {
					t.Errorf("parameter %d strict=%v: got %d, want %d", index, strict, got, want)
				}
			}
		}
	}
}

func TestSummaryBudgetPreservesOnlyMarkedPartialEvidence(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	function := pkg.Func("leaf")
	type evidence struct {
		witness int
		unknown bool
	}
	memo := ssaflow.NewCallGraphMemo[*ssa.Function, evidence]()
	computations := 0
	for _, limit := range []int{0, 1, 0} {
		budget := ssaflow.NewSearchBudget(limit)
		got := memo.Summarize(function, function, budget, func() evidence {
			computations++
			budget.Spend()
			return evidence{witness: 1}
		}, func(_ ssaflow.SummaryUnavailable, partial evidence) evidence {
			partial.unknown = true
			return partial
		})
		wantUnknown := computations == 1
		if got.witness != 1 || got.unknown != wantUnknown {
			t.Errorf("limit %d: got %+v, want witness with unknown=%v", limit, got, wantUnknown)
		}
	}
	if computations != 2 {
		t.Errorf("computations = %d, want 2 (partial discarded, complete retained)", computations)
	}
}

func TestSummaryRecursionGuardSpansContexts(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	function := pkg.Func("leaf")
	memo := ssaflow.NewCallGraphMemo[int, bool]()
	failed := func(reason ssaflow.SummaryUnavailable, _ bool) bool {
		if reason != ssaflow.SummaryRecursive {
			t.Errorf("unexpected cut reason: %v", reason)
		}
		return false
	}
	got := memo.Summarize(1, function, nil, func() bool {
		return memo.Summarize(2, function, nil, func() bool {
			t.Error("changing context bypassed the function recursion guard")
			return true
		}, failed)
	}, failed)
	if got {
		t.Fatal("recursive query produced a proof")
	}
	for _, context := range []int{1, 2} {
		if !memo.Summarize(context, function, nil, func() bool { return true }, failed) {
			t.Errorf("context %d retained a path-dependent cut", context)
		}
	}
}
