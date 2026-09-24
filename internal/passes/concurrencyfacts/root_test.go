package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestRootTracksBoundedChildren(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "rootworkers", `package rootworkers
func signal(ch chan int) { close(ch) }
func two(a, b chan int) { go signal(a); go signal(b) }
func emptyChild() { go func() {}() }
func five(a chan int) {
 go signal(a); go signal(a); go signal(a); go signal(a); go signal(a)
}
func optional(a chan int, run bool) { if run { go signal(a) } }
func unknown(a chan int, callback func()) { go signal(a); go callback() }
`)
	engine := NewEngine()
	function := pkg.Func("two")
	result := engine.Root(function, ssaflow.NewSearchBudget(2000))
	if !result.Complete() || len(result.Workers) != 2 || len(result.Operations) != 0 {
		t.Fatalf("two-child root = %+v", result)
	}
	for index, parameter := range function.Params[:2] {
		worker := result.Workers[index]
		if worker.Spawn == nil || worker.Prefix != 0 || len(worker.Operations) != 1 ||
			worker.Operations[0].Kind != Close || worker.Operations[0].Resource.Value != parameter {
			t.Errorf("child %d = %+v, want one close of %v", index, worker, parameter)
		}
	}
	empty := engine.Root(pkg.Func("emptyChild"), ssaflow.NewSearchBudget(2000))
	if !empty.Complete() || empty.Completeness() != CompleteWithEffects || len(empty.Workers) != 1 ||
		len(empty.Workers[0].Operations) != 0 {
		t.Errorf("event-free child was not retained: %+v", empty)
	}
	for name, reason := range map[string]string{
		"five":     "protocol-participants-unknown",
		"optional": "protocol-branch-effects-differ",
		"unknown":  "protocol-body-unavailable",
	} {
		t.Run(name, func(t *testing.T) {
			got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000))
			if got.Complete() || got.Reason != reason || len(got.Workers) != 0 {
				t.Errorf("%s root = %+v, want no usable children (%s)", name, got, reason)
			}
		})
	}
	if got := engine.Root(function, ssaflow.NewSearchBudget(1)); got.Complete() || len(got.Workers) != 0 {
		t.Fatalf("budget-cut root retained children: %+v", got)
	}
}
