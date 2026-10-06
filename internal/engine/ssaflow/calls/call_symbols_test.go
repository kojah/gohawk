package calls_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

func TestUnsafeBuiltinIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "intrinsics", `package intrinsics
import "unsafe"
func String(*byte, int) string { return "ordinary" }
func intrinsic(p *byte) string { return unsafe.String(p,1) }
func ordinary(p *byte) string { return String(p,1) }
`)
	for _, name := range []string{"intrinsic", "ordinary"} {
		call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name))[0]
		if got := ssacall.CallMatchesSymbol(call.Common(), syntax.Builtin("String")); got != (name == "intrinsic") {
			t.Errorf("%s matched unsafe builtin: %t", name, got)
		}
	}
}
