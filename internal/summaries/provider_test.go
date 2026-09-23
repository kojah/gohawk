package summaries

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestPassLevelSelection(t *testing.T) {
	for mask := range 8 {
		selection := Select(Requirements{Results: mask&1 != 0, Lifecycle: mask&2 != 0, Concurrency: mask&4 != 0})
		requires := selection.Requires()
		for index, component := range []*analysis.Analyzer{resultfacts.Analyzer, lifecyclefacts.Analyzer, concurrencyfacts.Analyzer} {
			if slices.Contains(requires, component) != (mask&(1<<index) != 0) {
				t.Errorf("selection %d: wrong prerequisite %s", mask, component.Name)
			}
		}
		if !selection.requirements.Concurrency {
			for _, prerequisite := range requires {
				assertNoConcurrencyDependency(t, prerequisite)
			}
		}
	}
}

func assertNoConcurrencyDependency(t *testing.T, analyzer *analysis.Analyzer) {
	t.Helper()
	if analyzer == concurrencyfacts.Analyzer {
		t.Fatal("unrequested concurrency inference scheduled")
	}
	for _, prerequisite := range analyzer.Requires {
		assertNoConcurrencyDependency(t, prerequisite)
	}
}

func TestAvailabilityIsPerComponent(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "broker", `package broker
func Yes() bool { return true }
func Unknown(value bool) bool { return value }
`)
	function := pkg.Func("Yes")
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		resultfacts.Analyzer:      resultfacts.NewEngine(),
		lifecyclefacts.Analyzer:   lifecyclefacts.Summaries{function: {}},
		concurrencyfacts.Analyzer: concurrencyfacts.NewEngine(),
	}}
	for _, requested := range []bool{false, true} {
		selection := Select(Requirements{Results: requested, Lifecycle: requested, Concurrency: requested})
		view := selection.Provider(pass).ForFunction(function)
		want := NotRequested
		if requested {
			want = Available
		}
		_, results := view.Results(ssaflow.NewSearchBudget(2000))
		_, lifecycle := view.Lifecycle()
		_, concurrency := view.Concurrency(ssaflow.NewSearchBudget(2000))
		if results != want || lifecycle != want || concurrency != want {
			t.Fatalf("availability: %v %v %v want %v", results, lifecycle, concurrency, want)
		}
	}
	selected := Select(Requirements{Results: true, Lifecycle: true, Concurrency: true})
	missing := selected.Provider(nil).ForFunction(function)
	_, results := missing.Results(ssaflow.NewSearchBudget(2000))
	_, lifecycle := missing.Lifecycle()
	_, concurrency := missing.Concurrency(ssaflow.NewSearchBudget(2000))
	if results != Unavailable || lifecycle != Unavailable || concurrency != Unavailable {
		t.Fatal("missing prerequisites treated as requested evidence")
	}
	unknown, available := selected.Provider(pass).ForFunction(pkg.Func("Unknown")).Results(ssaflow.NewSearchBudget(2000))
	if available != Available || unknown.Result(0) != resultfacts.Unknown {
		t.Fatal("available component confused with proven guarantee")
	}
}

func TestUnknownResultDoesNotPrune(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "broker", `package broker
func Unknown(value bool) bool { return value }
func Caller(value bool) int { if Unknown(value) { return 1 }; return 2 }
`)
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}}
	provider := Select(Requirements{Results: true}).Provider(pass)
	branch := ssaflow.InstructionsOf[*ssa.If](pkg.Func("Caller"))[0]
	if successors := provider.FeasibleSuccessors(branch.Block(), nil, ssaflow.NewSearchBudget(2000)); len(successors) != 2 {
		t.Fatalf("unknown eliminated a branch: %v", successors)
	}
}

// A result proven non-nil whenever its error is nil cannot be nil below the
// success arm of that error's check, so the nil-comparison branch is pruned.
// Above the check, or for a callee without the relation, both arms stay.
func TestFeasibleSuccessorsUseResultPairRelations(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "broker", `package broker
type box struct{}
type failure struct{}
func (*failure) Error() string { return "failure" }
func Open(ok bool) (*box, error) {
	if !ok {
		return nil, &failure{}
	}
	return &box{}, nil
}
func Loose(ok bool) (*box, error) {
	if !ok {
		return &box{}, &failure{}
	}
	return nil, nil
}
func guarded(ok bool) *box {
	b, err := Open(ok)
	if err != nil {
		return nil
	}
	if b == nil {
		return nil
	}
	return b
}
func unguarded(ok bool) *box {
	b, _ := Open(ok)
	if b == nil {
		return nil
	}
	return b
}
func loose(ok bool) *box {
	b, err := Loose(ok)
	if err != nil {
		return nil
	}
	if b == nil {
		return nil
	}
	return b
}
`)
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}}
	provider := Select(Requirements{Results: true}).Provider(pass)
	for name, want := range map[string]int{"guarded": 1, "unguarded": 2, "loose": 2} {
		function := pkg.Func(name)
		var block *ssa.BasicBlock
		for _, candidate := range function.Blocks {
			if branch, ok := candidate.Instrs[len(candidate.Instrs)-1].(*ssa.If); ok {
				if comparison, ok := branch.Cond.(*ssa.BinOp); ok && comparison.X.Type().String() == "*broker.box" {
					block = candidate
				}
			}
		}
		if block == nil {
			t.Fatalf("%s: no nil comparison of the result", name)
		}
		var predecessor *ssa.BasicBlock
		if len(block.Preds) > 0 {
			predecessor = block.Preds[0]
		}
		got := provider.FeasibleSuccessors(block, predecessor, ssaflow.NewSearchBudget(2000))
		if len(got) != want {
			t.Errorf("%s: %d feasible successors, want %d", name, len(got), want)
		}
	}
}
