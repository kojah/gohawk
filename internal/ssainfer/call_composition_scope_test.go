package ssainfer_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

const compositionScopeFixture = `package scopes
func first() {}
func second() {}
func opaque()
`

func TestSummaryCompositionVisitsIndependentBodies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "scopes", compositionScopeFixture)
	memo := ssaflow.NewCallGraphMemo[string, int]()
	visits := 0
	compute := func() int {
		for _, name := range []string{"first", "second", "first"} {
			if !memo.WithFunction(pkg.Func(name), func() { visits++ }) {
				t.Errorf("sequential body %s rejected; prior scope was not released", name)
			}
		}
		return visits
	}
	for range 2 {
		got := memo.Compose("call-site question", ssaflow.NewSearchBudget(10), compute,
			func(ssaflow.SummaryUnavailable, int) int { return -1 })
		if got != 3 || visits != 3 {
			t.Errorf("answer=%d visits=%d, want 3 and 3", got, visits)
		}
	}
	for _, name := range []string{"missing", "opaque"} {
		if memo.WithFunction(pkg.Func(name), func() { t.Error("opaque body visited") }) {
			t.Errorf("%s reported a visited body", name)
		}
	}
}

func TestSummaryCompositionRecursiveAlternativeInvalidatesQuestion(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "scopes", compositionScopeFixture)
	memo := ssaflow.NewCallGraphMemo[string, int]()
	unavailable := func(ssaflow.SummaryUnavailable, int) int { return -1 }
	visits := 0
	budget := ssaflow.NewSearchBudget(10)
	for range 2 {
		got := memo.Compose("multiple callees", budget, func() int {
			memo.WithFunction(pkg.Func("first"), func() {
				visits++
				memo.Compose("nested question", budget, func() int {
					if memo.WithFunction(pkg.Func("first"), func() { t.Error("recursive body visited") }) {
						t.Error("recursive alternative accepted")
					}
					return 5 // Positive witness, not an exhaustive summary.
				}, unavailable)
			})
			return 5
		}, unavailable)
		if got != 5 {
			t.Errorf("independent witness lost: %d", got)
		}
	}
	if visits != 2 {
		t.Errorf("path-dependent answer cached: visits=%d", visits)
	}
	if got := memo.Compose("nested question", budget, func() int { return 9 }, unavailable); got != 9 {
		t.Errorf("nested cut poisoned its cache: %d", got)
	}
}

func TestSummaryCompositionPolicyTruncationInvalidatesParents(t *testing.T) {
	memo := ssaflow.NewCallGraphMemo[int, int]()
	unavailable := func(ssaflow.SummaryUnavailable, int) int { return -1 }
	budget := ssaflow.NewSearchBudget(10)
	memo.Compose(1, budget, func() int {
		return memo.Compose(2, budget, func() int {
			memo.Incomplete()
			return 7
		}, unavailable)
	}, unavailable)
	for _, key := range []int{1, 2} {
		if got := memo.Compose(key, budget, func() int { return 9 }, unavailable); got != 9 {
			t.Errorf("question %d retained incomplete answer %d", key, got)
		}
	}
}
