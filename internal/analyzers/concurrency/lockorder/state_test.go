package lockorder

import (
	"go/ast"
	"go/constant"
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
			constants: []lockScalarConstant{{value: phi, literal: ssa.NewConst(constant.MakeBool(true), types.Typ[types.Bool])}},
		}
		got := lockPhiConstants(state)
		_, literal := incoming.(*ssa.Const)
		truth, known := lockBooleanValue(phi, got)
		if known != literal || truth {
			t.Errorf("incoming %T: got truth=%t known=%t, want false/%t", incoming, truth, known, literal)
		}
		if !constant.BoolVal(state.constants[0].literal.Value) {
			t.Error("entry transfer mutated predecessor state")
		}
	}
}

func TestLockLiteralBranch(t *testing.T) {
	phase := &ssa.Phi{}
	initial := ssa.NewConst(constant.MakeInt64(-1), types.Typ[types.Int])
	other := ssa.NewConst(constant.MakeInt64(2), types.Typ[types.Int])
	bindings := []lockScalarConstant{{value: phase, literal: initial}}
	for _, test := range []struct {
		name  string
		value ssa.Value
		truth bool
		known bool
	}{
		{"equal", &ssa.BinOp{Op: token.EQL, X: phase, Y: initial}, true, true},
		{"unequal", &ssa.BinOp{Op: token.NEQ, X: phase, Y: initial}, false, true},
		{"different", &ssa.BinOp{Op: token.NEQ, X: phase, Y: other}, true, true},
		{"arithmetic", &ssa.BinOp{Op: token.ADD, X: phase, Y: other}, false, false},
		{"unknown", &ssa.BinOp{Op: token.EQL, X: &ssa.Phi{}, Y: initial}, false, false},
		{"ordering", &ssa.BinOp{Op: token.LSS, X: phase, Y: other}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			truth, known := lockBooleanValue(test.value, bindings)
			if truth != test.truth || known != test.known {
				t.Errorf("got %t/%t, want %t/%t", truth, known, test.truth, test.known)
			}
		})
	}
}
