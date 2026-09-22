package lockorder

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func TestLockPhiConstantsForgetUnknownInput(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "state.go", `package state
func merge(choose, unknown bool) bool {
    value := false
    if choose { value = unknown }
    return value
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssautil.BuildPackage(&types.Config{}, fset, types.NewPackage("state", "state"), []*ast.File{file}, ssa.SanityCheckFunctions)
	if err != nil {
		t.Fatal(err)
	}
	phis := ssaflow.InstructionsOf[*ssa.Phi](pkg.Func("merge"))
	if len(phis) != 1 {
		t.Fatalf("got %d phis, want one", len(phis))
	}
	phi := phis[0]
	for predecessor, incoming := range ssaflow.PhiIncoming(phi) {
		state := lockFlowState{
			block: phi.Block(), predecessor: predecessor,
			constants: []lockBooleanConstant{{value: phi, truth: true}},
		}
		got := lockPhiConstants(state)
		_, literal := incoming.(*ssa.Const)
		truth, known := lockBooleanValue(phi, got)
		if known != literal || truth {
			t.Errorf("incoming %T: got truth=%t known=%t, want false/%t", incoming, truth, known, literal)
		}
		if !state.constants[0].truth {
			t.Error("entry transfer mutated predecessor state")
		}
	}
}
