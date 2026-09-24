package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

// Each path alternative records the branch choices that select it. A helper's
// alternatives keep their own conditions, tagged with the call site, so two
// calls of one helper never share a condition.
func TestPathConditions(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "conditions", `package conditions
func branch(a, b chan int, flag bool) { if flag { close(a) } else { close(b) } }
func twice(a, b chan int, x, y bool) { branch(a, b, x); branch(a, b, y) }
func worker(a, b chan int, flag bool) { go branch(a, b, flag) }
`)
	engine := NewEngine()
	budget := func() *ssaflow.SearchBudget { return ssaflow.NewSearchBudget(4000) }
	function := pkg.Func("branch")
	got := engine.Function(function, budget())
	if len(got.Paths) != 2 {
		t.Fatalf("branch = %+v, want two paths", got)
	}
	for _, path := range got.Paths {
		if len(path.Conditions) != 1 || path.Conditions[0].Value != function.Params[2] || len(path.Conditions[0].Context) != 0 {
			t.Fatalf("path conditions = %+v, want one local condition on flag", path.Conditions)
		}
		closesA := path.Operations[0].Resource.Value == function.Params[0]
		if path.Conditions[0].Holds != closesA {
			t.Errorf("path %+v: condition polarity does not select its effects", path)
		}
	}
	twice := engine.Function(pkg.Func("twice"), budget())
	if len(twice.Paths) != 4 {
		t.Fatalf("twice = %+v, want four combined paths", twice)
	}
	for _, path := range twice.Paths {
		if len(path.Conditions) != 2 || len(path.Conditions[0].Context) != 1 || len(path.Conditions[1].Context) != 1 ||
			path.Conditions[0].Context[0] == path.Conditions[1].Context[0] {
			t.Errorf("twice path conditions = %+v, want one per call, tagged with distinct sites", path.Conditions)
		}
	}
	launched := engine.Root(pkg.Func("worker"), budget())
	if len(launched.Workers) != 1 || len(launched.Workers[0].AlternativeConditions) != 2 {
		t.Fatalf("worker = %+v, want two conditioned alternatives", launched)
	}
	for _, conditions := range launched.Workers[0].AlternativeConditions {
		if len(conditions) != 1 || len(conditions[0].Context) != 1 {
			t.Errorf("worker alternative conditions = %+v, want one bound condition", conditions)
		}
	}
}
