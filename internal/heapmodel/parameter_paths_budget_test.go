package heapmodel

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestParameterSpillPathsShareAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spillpaths", `package spillpaths
 type box struct { first,second *int }
 func expose(*box)
 func direct(b *box)*int{return b.first}
 func spilled(b box)*int{return b.first}
 func copied(b box)*int{k:=b;return k.second}
 func indexed(b [2]*int)*int{return b[1]}
 func replaced(b box,p *int)*int{b.first=p;return b.first}
 func escaped(b box)*int{expose(&b);return b.first}
 func dynamic(b []*int,i int)*int{return b[i]}
 func unrelated(b box,p *int)*int{var k box;k.first=p;return k.first}
 `)
	for _, test := range []struct {
		name  string
		path  []string
		known bool
	}{
		{"direct", []string{"field:0"}, true},
		{"spilled", []string{"field:0"}, true},
		{"copied", []string{"field:1"}, true},
		{"indexed", []string{"index:1"}, true},
		{"replaced", nil, false},
		{"escaped", nil, false},
		{"dynamic", nil, false},
		{"unrelated", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
			baseline, known := AccessPathFromParameter(returned, fn.Params[0])
			if known != test.known || !slices.Equal(baseline, test.path) {
				t.Fatalf("default path=%v/%v", baseline, known)
			}
			if known {
				cut := proofs.NewSearchBudget(1)
				path, ok := AccessPathFromParameterWithin(returned, fn.Params[0], cut)
				if ok || path != nil || !cut.Exhausted() {
					t.Fatalf("path bypassed caller allowance: %v/%v", path, ok)
				}
			}
			for limit := 1; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				path, ok := AccessPathFromParameterWithin(returned, fn.Params[0], budget)
				if budget.Exhausted() {
					if ok || path != nil {
						t.Fatalf("cut %d publishes path=%v/%v", limit, path, ok)
					}
					continue
				}
				if ok != known || !slices.Equal(path, baseline) {
					t.Fatalf("completed %d path=%v/%v", limit, path, ok)
				}
				return
			}
			t.Fatal("path query never completed")
		})
	}
}

func TestWholeWrittenSpillCellAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spillcell", `package spillcell
 type box struct {first *int}
 func spilled(b box)*int{return b.first}
 `)
	fn := pkg.Func("spilled")
	cells := ssaflow.InstructionsOf[*ssa.Alloc](fn)
	if len(cells) != 1 {
		t.Fatalf("spill cell count=%d", len(cells))
	}
	pool := proofs.NewSearchBudget(proofs.QueryBudget)
	cut := pool.Within(1)
	if ssaflow.WholeWrittenCellWithin(cells[0], cut) || !cut.Exhausted() || pool.Exhausted() {
		t.Fatal("whole-cell cutoff not retained")
	}
	if !ssaflow.WholeWrittenCellWithin(cells[0], pool.Within(proofs.QueryBudget)) {
		t.Fatal("fresh whole-cell query did not recover")
	}
}
