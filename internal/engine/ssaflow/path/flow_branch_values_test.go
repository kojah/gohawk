package path_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestBranchValuePreservesPathAndBlockIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "branchvalues", `package branchvalues
func saved(a, b bool) bool { condition := a && b; if condition { return true }; return false }
func later(a, b, gate bool) bool {
	condition := a && b
	if gate { gate = false }
	if condition { return gate }
	return false
}
`)
	for _, name := range []string{"saved", "later"} {
		function := pkg.Func(name)
		phis := ssaflow.InstructionsOf[*ssa.Phi](function)
		if len(phis) == 0 {
			t.Fatalf("%s has no saved-condition phi", name)
		}
		phi := phis[0]
		if got := ssapath.BranchValueWithin(phi, phi.Block(), nil, nil); got != phi {
			t.Fatalf("missing predecessor selected %v", got)
		}
		if got := ssapath.BranchValueWithin(phi, phi.Block(), &ssa.BasicBlock{}, nil); got != phi {
			t.Fatalf("unrelated predecessor selected %v", got)
		}
		for predecessor, operand := range ssaflow.PhiIncoming(phi) {
			if got := ssapath.BranchValueWithin(phi, phi.Block(), predecessor, nil); got != operand {
				t.Fatalf("%s: incoming value = %v, want %v", name, got, operand)
			}
			if got := ssapath.BranchValueWithin(phi, function.Blocks[0], predecessor, nil); got != phi {
				t.Fatalf("%s: historical phi selected %v in a different block", name, got)
			}
		}
	}
}
