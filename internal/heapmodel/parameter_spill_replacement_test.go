package heapmodel

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestSpillPathsRequireOriginalContentsAtRead(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spillreplacement", `package spillreplacement
 type box struct {first *int; child *box}
 func replaced(b,c box)*int{b=c;return b.first}
 func earlier(b,c box)*int{p:=b.first;b=c;return p}
 func wrapped(b,c box)interface{}{p:=b.first;b=c;return p}
 func restoredAfterRead(b,c box)*int{original:=b;b=c;p:=b.first;b=original;return p}
 func nestedEarlier(b,c box)*int{child:=b.child;b=c;return child.first}
 func agreeing(b box,flag bool)*int{original:=b;if flag{b=original};return b.first}
 func ambiguous(b,c box,flag bool)*int{if flag{b=c};return b.first}
 func addressBefore(b,c box)**int{p:=&b.first;b=c;return p}
 `)
	for _, test := range []struct {
		name      string
		parameter int
		path      []string
		known     bool
	}{
		{"replaced", 0, nil, false},
		{"replaced", 1, []string{"field:0"}, true},
		{"earlier", 0, []string{"field:0"}, true},
		{"earlier", 1, nil, false},
		{"wrapped", 0, []string{"field:0"}, true},
		{"restoredAfterRead", 0, nil, false},
		{"restoredAfterRead", 1, []string{"field:0"}, true},
		{"nestedEarlier", 0, []string{"field:1", "field:0"}, true},
		{"agreeing", 0, []string{"field:0"}, true},
		{"ambiguous", 0, nil, false},
		{"ambiguous", 1, nil, false},
		{"addressBefore", 0, nil, false},
	} {
		fn := pkg.Func(test.name)
		value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
		path, known := AccessPathFromParameterWithin(value, fn.Params[test.parameter], ssaflow.NewSearchBudget(ssaflow.QueryBudget))
		regionGraphs.Lock()
		_, built := regionGraphs.entries[fn]
		regionGraphs.Unlock()
		if built {
			t.Fatalf("%s path discovery built a graph", test.name)
		}
		if known != test.known || !slices.Equal(path, test.path) {
			t.Errorf("%s parameter%d path=%v/%v, want%v/%v", test.name, test.parameter, path, known, test.path, test.known)
		}
		completed := false
		for limit := 1; limit <= ssaflow.QueryBudget; limit++ {
			budget := ssaflow.NewSearchBudget(limit)
			path, known := AccessPathFromParameterWithin(value, fn.Params[test.parameter], budget)
			if budget.Exhausted() {
				if known || path != nil {
					t.Fatalf("%s cutoff %d published path %v", test.name, limit, path)
				}
				continue
			}
			if known != test.known || !slices.Equal(path, test.path) {
				t.Fatalf("%s fresh allowance %d path=%v/%v", test.name, limit, path, known)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatalf("%s never completed within allowance", test.name)
		}
	}
}
