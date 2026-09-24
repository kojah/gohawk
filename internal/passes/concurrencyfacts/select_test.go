package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func selectEffectsPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "selecteffects", `package selecteffects
func worker(out chan int, cancel <-chan int) {
 select { case out <- 1: case <-cancel: return }
}
func root(out chan int, cancel <-chan int) { go worker(out, cancel) }
func defaultArm(out chan int) { select { case out <- 1: default: } }
func conditional(out, done chan int, flag bool) {
 select { case out <- 1: case out <- 2: }
 if flag { <-done }
}
func uniform(out chan int) { select { case out <- 1: case out <- 2: } }
func continuing(a, b chan int) { select { case a <- 1: case a <- 2: }; <-b }
func continuingRoot(a, b chan int) { go continuing(a, b); b <- 1; <-a }
func helper(out chan int, cancel <-chan int) { worker(out, cancel) }
`)
}

func TestSelectAlternativesRemainExclusive(t *testing.T) {
	pkg := selectEffectsPackage(t)
	engine := NewEngine()
	for _, name := range []string{"worker", "defaultArm", "helper"} {
		function := pkg.Func(name)
		result := engine.Function(function, ssaflow.NewSearchBudget(2000))
		if result.Complete() || result.Reason != "protocol-select-alternatives" || len(result.Choices) != 1 {
			t.Fatalf("%s = %+v, want exclusive alternatives", name, result)
		}
		choice := result.Choices[0]
		if len(choice.Arms) != 2 || choice.Arms[0].Operation.Kind != Send ||
			(choice.Arms[1].Operation.Kind != Receive && !choice.Arms[1].Default) {
			t.Errorf("%s choice = %+v", name, choice)
		}
		if _, exported := exportSummary(function, result); exported {
			t.Errorf("%s exported conditional effects as a linear fact", name)
		}
	}
	uniform := engine.Function(pkg.Func("uniform"), ssaflow.NewSearchBudget(2000))
	if uniform.Complete() || uniform.Reason != "protocol-select-alternatives" || len(uniform.Choices) != 1 ||
		len(uniform.Choices[0].Arms) != 2 {
		t.Errorf("uniform select = %+v", uniform)
	}
}

func TestSelectWorkerResourcesBindToCaller(t *testing.T) {
	pkg := selectEffectsPackage(t)
	engine := NewEngine()
	worker := engine.Root(pkg.Func("root"), ssaflow.NewSearchBudget(2000))
	if worker.Complete() || worker.Reason != "protocol-select-alternatives" ||
		!worker.AlternativesComplete || len(worker.Workers) != 1 || len(worker.Workers[0].Alternatives) != 2 ||
		len(worker.Choices) != 1 || worker.Choices[0].Worker == nil ||
		worker.Choices[0].Arms[0].Operation.Resource.Value != pkg.Func("root").Params[0] ||
		worker.Choices[0].Arms[1].Operation.Resource.Value != pkg.Func("root").Params[1] {
		t.Fatalf("child select = %+v", worker)
	}
}

func TestSelectContinuationsAreBoundToWorker(t *testing.T) {
	pkg := selectEffectsPackage(t)
	engine := NewEngine()
	continuing := engine.Function(pkg.Func("continuing"), ssaflow.NewSearchBudget(2000))
	if !continuing.AlternativesComplete || len(continuing.Choices) != 1 ||
		len(continuing.Choices[0].Arms[0].Sequence) != 2 || len(continuing.Choices[0].Arms[1].Sequence) != 2 {
		t.Errorf("select continuations = %+v", continuing)
	}
	root := engine.Root(pkg.Func("continuingRoot"), ssaflow.NewSearchBudget(2000))
	if !root.AlternativesComplete || len(root.Workers) != 1 || len(root.Workers[0].Alternatives) != 2 ||
		len(root.Operations) != 2 {
		t.Errorf("worker continuations = %+v", root)
	}
}

func TestSelectDefaultAndUnknownBranch(t *testing.T) {
	pkg := selectEffectsPackage(t)
	engine := NewEngine()
	defaultResult := engine.Function(pkg.Func("defaultArm"), ssaflow.NewSearchBudget(2000))
	if !defaultResult.AlternativesComplete || !defaultResult.Choices[0].Arms[1].Complete ||
		len(defaultResult.Choices[0].Arms[1].Sequence) != 0 {
		t.Errorf("default continuation = %+v", defaultResult)
	}
	conditional := engine.Function(pkg.Func("conditional"), ssaflow.NewSearchBudget(2000))
	if conditional.AlternativesComplete || conditional.Complete() {
		t.Errorf("unrelated branch was treated as a select continuation: %+v", conditional)
	}
}
