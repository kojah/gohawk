package heapmodel

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestRegionContainmentObservation(t *testing.T) {
	for _, test := range []struct {
		name, body             string
		history, before, after bool
	}{
		{"laterStore", `x := node{}; observe(&x,a); x.value=a; observe(&x,a)`, true, false, true},
		{"replacedStore", `x := node{value:a}; observe(&x,a); x.value=b; observe(&x,a)`, true, true, false},
		{"unrelated", `x := node{value:b}; observe(&x,a); x.value=b; observe(&x,a)`, false, false, false},
		{"nested", `x,y := node{},node{}; x.next=&y; observe(&x,a); y.value=a; observe(&x,a)`, true, false, true},
		{"cycleApart", `x := node{}; x.next=&x; observe(&x,a); observe(&x,a)`, false, false, false},
		{"cycleContains", `x := node{value:a}; x.next=&x; observe(&x,a); observe(&x,a)`, true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkRegionContainment(t, test.body, test.history, test.before, test.after)
		})
	}
}

func TestRegionContainmentDepth(t *testing.T) {
	for _, links := range []int{aliasDepth - 1, aliasDepth} {
		t.Run(fmt.Sprintf("links%d", links), func(t *testing.T) {
			var body strings.Builder
			for index := range links + 1 {
				fmt.Fprintf(&body, "n%d := node{}; ", index)
			}
			for index := range links {
				fmt.Fprintf(&body, "n%d.next=&n%d; ", index, index+1)
			}
			fmt.Fprintf(&body, "n%d.value=a; observe(&n0,a); observe(&n0,a)", links)
			// Each node link consumes one level; reaching its value consumes another.
			want := links < aliasDepth
			checkRegionContainment(t, body.String(), want, want, want)
		})
	}
}

func checkRegionContainment(t *testing.T, body string, history, before, after bool) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "containmentprobe", `package containmentprobe
type node struct { next *node; value *int }
func observe(a,b any) {}
func probe(a,b *int) { `+body+` }
`)
	var observations []*ssa.Call
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("probe")) {
		if ssaflow.CallName(call.Common()) == "observe" {
			observations = append(observations, call)
		}
	}
	if len(observations) != 2 {
		t.Fatalf("want two observations, got %d", len(observations))
	}
	for index, observation := range observations {
		owner, target := unwrapInterface(observation.Common().Args[0]), unwrapInterface(observation.Common().Args[1])
		if got := Contains(owner, target); got != history {
			t.Errorf("observation %d: history = %t, want %t", index, got, history)
		}
		want := []bool{before, after}[index]
		if got, known := ContainsAt(owner, target, observation); !known || got != want {
			t.Errorf("observation %d: at = %t, known = %t, want %t", index, got, known, want)
		}
		if got, known := ContainsAt(owner, target, nil); known || got {
			t.Errorf("missing observation must remain unknown, got %t, known = %t", got, known)
		}
	}
}
