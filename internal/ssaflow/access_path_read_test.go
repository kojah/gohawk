package ssaflow

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAccessPathReadKeepsSnapshotAndDiscardsCutMetadata(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "reads", `package reads
 type box struct{child *box; value *int}
 func saved(b,c box)*int{child:=b.child;b=c;return child.value}
 `)
	fn := pkg.Func("saved")
	cells := InstructionsOf[*ssa.Alloc](fn)
	loads := InstructionsOf[*ssa.UnOp](fn)
	if len(cells) != 1 || len(loads) != 2 {
		t.Fatal("expected one spill and two field reads")
	}
	value := InstructionsOf[*ssa.Return](fn)[0].Results[0]
	sawCut := false
	for limit := 1; limit <= QueryBudget; limit++ {
		budget := NewSearchBudget(limit)
		path, read, known := AccessPathReadWithin(value, cells[0], budget)
		if budget.Exhausted() {
			sawCut = true
			if known || path != nil || read != nil {
				t.Fatalf("cutoff %d published %v/%v/%v", limit, path, read, known)
			}
			continue
		}
		if !sawCut || !known || read != loads[0] || !slices.Equal(path, []string{"field:0", "field:1"}) {
			t.Fatalf("fresh allowance %d path=%v read=%v known=%v", limit, path, read, known)
		}
		return
	}
	t.Fatal("path query did not recover")
}
