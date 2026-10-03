package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStructuralSameAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "samebudget", `package samebudget
func directed(ch chan int) (chan<- int, <-chan int) { return ch, ch }
func stored(value, other *int, replace bool) *int {
 selected := value
 func() { println(selected) }()
 if replace { selected = other }
 return selected
}
func indexed(value *[2]int, index int) (*int, *int) { return &value[index], &value[index] }
`)
	for _, name := range []string{"directed", "stored", "indexed"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			left := returned.Results[0]
			var right ssa.Value
			if name == "stored" {
				right = fn.Params[0]
			} else {
				right = returned.Results[1]
			}
			if !ssaflow.StructurallySame(left, right) {
				t.Fatal("fixture must establish possible structural identity")
			}
			completed := false
			for limit := 0; limit <= ssaflow.QueryBudget; limit++ {
				budget := ssaflow.NewSearchBudget(limit)
				got := ssaflow.StructurallySameWithin(left, right, budget)
				if budget.Exhausted() {
					if got {
						t.Fatalf("allowance %d supplied interrupted identity", limit)
					}
					continue
				}
				if !got {
					t.Fatal("fresh query lost possible identity")
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("identity query never completed")
			}
		})
	}
}
