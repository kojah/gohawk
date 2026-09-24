package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestSelectAlternativesRemainExclusive(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "selecteffects", `package selecteffects
func worker(out chan int, cancel <-chan int) {
 select { case out <- 1: case <-cancel: return }
}
func root(out chan int, cancel <-chan int) { go worker(out, cancel) }
func defaultArm(out chan int) { select { case out <- 1: default: } }
func uniform(out chan int) { select { case out <- 1: case out <- 2: } }
func helper(out chan int, cancel <-chan int) { worker(out, cancel) }
`)
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
	worker := engine.Root(pkg.Func("root"), ssaflow.NewSearchBudget(2000))
	if worker.Complete() || worker.Reason != "protocol-select-alternatives" ||
		len(worker.Choices) != 1 || worker.Choices[0].Worker == nil ||
		worker.Choices[0].Arms[0].Operation.Resource.Value != pkg.Func("root").Params[0] ||
		worker.Choices[0].Arms[1].Operation.Resource.Value != pkg.Func("root").Params[1] {
		t.Fatalf("child select = %+v", worker)
	}
	uniform := engine.Function(pkg.Func("uniform"), ssaflow.NewSearchBudget(2000))
	if uniform.Complete() || uniform.Reason != "protocol-select-alternatives" || len(uniform.Choices) != 1 ||
		len(uniform.Choices[0].Arms) != 2 {
		t.Errorf("uniform select = %+v", uniform)
	}
}
