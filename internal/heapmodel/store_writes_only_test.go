package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestContentFromWritesDoesNotBuildGraph(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "writequery", `package writequery
type box struct { flag bool; next *box }
func opaque(*box)
func stable() bool { b := &box{flag: true}; return b.flag }
func escaped() bool { b := &box{flag: true}; opaque(b); return b.flag }
func nested() bool { b := &box{next: &box{flag: true}}; opaque(b); return b.next.flag }
func mixed(pick bool) bool { b := &box{flag: true}; if pick { b.flag = false }; return b.flag }
func pointerMixed(p,q *box,pick bool) *box { b := &box{next:p}; if pick { b.next=q }; return b.next }
`)
	for _, name := range []string{"stable", "escaped", "nested", "mixed", "pointerMixed"} {
		function := pkg.Func(name)
		loads := ssaflow.InstructionsOf[*ssa.UnOp](function)
		load := loads[len(loads)-1]
		proof := NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget)).ContentFromWrites(load.X, load)
		if proof.Proven() != (name == "stable") {
			t.Errorf("%s: unexpected proof %+v", name, proof)
		}
		regionGraphs.Lock()
		_, built := regionGraphs.entries[function]
		regionGraphs.Unlock()
		if built {
			t.Errorf("%s: writes-only query requested a points-to graph", name)
		}
	}
}
