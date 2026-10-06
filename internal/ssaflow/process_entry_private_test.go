package ssaflow_test

import (
	"fmt"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestPrivateEntryChain(t *testing.T) {
	for _, test := range []struct {
		name, helper, main, extra string
		want                      bool
	}{
		{"init", "func helper(){acquire()}", "", "var value=func()int{helper();return 1}()", false},
		{"generic", "func helper[T any](){acquire()}", "helper[int]()", "", false},
		{"single", "func helper(){acquire()}", "helper()", "", true},
		{"chain", "func helper(){acquire()};func bridge(){helper()}", "bridge()", "", true},
		{"twice", "func helper(){acquire()}", "helper();helper()", "", false},
		{"loop", "func helper(){acquire()}", "for {helper()}", "", false},
		{"inner-loop", "func helper(){for {acquire()}}", "helper()", "", false},
		{"callback", "func helper(){acquire()}", "helper();consume(helper)", "func consume(func()){}", false},
		{"async", "func helper(){acquire()}", "helper();go helper()", "", false},
		{"deferred", "func helper(){acquire()}", "helper();defer helper()", "", false},
		{"alias", "func helper(){acquire()}", "helper()", "var alias=helper", false},
		{"reentry", "func helper(){acquire()}", "helper()", "var alias=main", false},
		{"recursion", "func helper(){acquire();helper()}", "helper()", "", false},
		{"unused", "func helper(){acquire()}", "", "", false},
		{"exported", "func Helper(){acquire()}", "Helper()", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "main", "package main;func acquire(){};"+test.helper+";func main(){"+test.main+"};"+test.extra)
			name := "helper"
			if test.name == "exported" {
				name = "Helper"
			}
			var call *ssa.Call
			for _, c := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name)) {
				if ssaflow.CallName(c.Common()) == "acquire" {
					call = c
				}
			}
			if call == nil {
				t.Fatal("missing acquisition")
			}
			if got := ssaflow.RunsOnceThroughPrivateEntryCallsWithin(call, proofs.NewSearchBudget(20000)); got != test.want {
				t.Errorf("got %v want %v", got, test.want)
			}
		})
	}
}

func TestPrivateEntryChainCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "main", `package main
 func acquire(){};func leaf(){acquire()};func bridge(){leaf()};func main(){bridge()}`)
	call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("leaf"))[0]
	finished := false
	for limit := range 1000 {
		pool := proofs.NewSearchBudget(20000)
		budget := pool.Within(limit)
		got := ssaflow.RunsOnceThroughPrivateEntryCallsWithin(call, budget)
		if got {
			if budget.Exhausted() {
				t.Fatal("interrupted census proved entry chain")
			}
			finished = true
			break
		}
		if !budget.Exhausted() || pool.Exhausted() {
			t.Fatalf("cut %d lost allowance ownership", limit)
		}
		if !ssaflow.RunsOnceThroughPrivateEntryCallsWithin(call, pool.Within(20000)) {
			t.Fatalf("cut %d contaminated fresh query", limit)
		}
	}
	if !finished {
		t.Fatal("entry chain never completed")
	}
}

func TestPrivateEntryChainDepth(t *testing.T) {
	for _, length := range []int{14, 15} {
		var source strings.Builder
		source.WriteString("package main;func acquire(){};func helper(){acquire()};")
		previous := "helper"
		for index := range length {
			name := fmt.Sprintf("bridge%d", index)
			fmt.Fprintf(&source, "func %s(){%s()};", name, previous)
			previous = name
		}
		fmt.Fprintf(&source, "func main(){%s()}", previous)
		pkg := ssaflowtest.BuildPackage(t, "main", source.String())
		call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("helper"))[0]
		if got := ssaflow.RunsOnceThroughPrivateEntryCallsWithin(call, proofs.NewSearchBudget(20000)); got != (length == 14) {
			t.Errorf("%d bridges: %v", length, got)
		}
	}
}
