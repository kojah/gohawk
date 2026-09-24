package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestBoundedBranchPaths(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "paths", `package paths
func branch(a, b chan int, flag bool) { if flag { close(a) } else { close(b) } }
func forward(a, b chan int, flag bool) { branch(a, b, flag) }
func launch(a, b chan int, flag bool) { go forward(a, b, flag) }
func selectAndLaunch(a, b chan int) { go func(){ close(a) }(); select { case <-b: default: } }
func selectCaller(a, b chan int) { selectAndLaunch(a, b) }
func nested(a, b chan int) { go selectAndLaunch(a, b) }
func armLaunch(a, b chan int) { select { case <-b: go func(){ close(a) }(); default: close(a) } }
func launchAndClose(a, b chan int) { go func(){ close(a) }(); close(b) }
func deferredLaunch(a, b chan int) { defer launchAndClose(a, b) }
func optional(a chan int, flag bool) { if flag { close(a) } }
func opaque(a chan int, flag bool, f func()) { if flag { close(a) } else { f() } }
func loop(a chan int, flag bool) { for flag { close(a) } }
func many(a chan int, x, y, z, w bool) {
 if x { close(a) }; if y { close(a) }; if z { close(a) }; if w { close(a) }
}
`)
	engine := NewEngine()
	linear := engine.linear.Function(pkg.Func("branch"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if linear.Reason != "protocol-branch-effects-differ" || len(linear.Paths) != 0 {
		t.Fatalf("linear export built unpublishable paths: %+v", linear)
	}
	launch := engine.Root(pkg.Func("launch"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if len(launch.Workers) != 1 || !launch.Workers[0].Branches || len(launch.Workers[0].Alternatives) != 2 {
		t.Fatalf("forwarded worker alternatives = %+v", launch)
	}
	for _, name := range []string{"branch", "forward", "optional"} {
		got := engine.Function(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if got.Complete() || len(got.Paths) != 2 || got.Reason != "protocol-branch-alternatives" {
			t.Errorf("%s = %+v, want two non-linear paths", name, got)
		}
		for _, path := range got.Paths {
			if !path.Complete() || len(path.Paths) != 0 {
				t.Errorf("%s has incomplete or nested path: %+v", name, path)
			}
		}
	}
	optional := engine.Function(pkg.Func("optional"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if len(optional.Paths) == 2 && len(optional.Paths[0].Operations)+len(optional.Paths[1].Operations) != 1 {
		t.Error("optional cleanup lost its empty escape path")
	}
	for _, name := range []string{"opaque", "loop", "many", "selectCaller", "nested", "armLaunch", "deferredLaunch"} {
		got := engine.Function(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if got.Complete() || len(got.Paths) != 0 {
			t.Errorf("%s must remain unavailable, got %+v", name, got)
		}
	}
}
