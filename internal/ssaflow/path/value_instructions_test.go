package path_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReturnAndActionWitnesses(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "witnesses", `package witnesses
func mark(value int) {}
func missingAction() {}
func missingReturn() { mark(0); for {} }
func earlyAction() { mark(1); mark(2) }
func optionalAction(flag bool) { if flag { mark(3) } }
`)
	mark := func(instruction ssa.Instruction) bool {
		call, ok := instruction.(*ssa.Call)
		return ok && call.Common().StaticCallee() == pkg.Func("mark")
	}
	for name, want := range map[string]bool{
		"missingAction": false, "missingReturn": false, "earlyAction": true, "optionalAction": true,
	} {
		if got := ssaflow.HasReturnAndAction(pkg.Func(name).Blocks, mark); got != want {
			t.Errorf("%s: got witnesses %v, want %v", name, got, want)
		}
	}
	optional := pkg.Func("optionalAction")
	blocks := ssapath.ReachableBlocksAssumingWithin(optional, ssacall.FixedValues{optional.Params[0]: ssacall.OutcomeFalse}, nil)
	if ssaflow.HasReturnAndAction(blocks, mark) || ssaflow.HasReturnAndAction(nil, mark) {
		t.Error("filtered or absent blocks supplied an action witness")
	}
	// A first action must not stop the return scan or cause further predicate
	// calls. This matters when the predicate spends a shared query budget.
	queries := 0
	if !ssaflow.HasReturnAndAction(pkg.Func("earlyAction").Blocks, func(instruction ssa.Instruction) bool {
		queries++
		return mark(instruction)
	}) || queries != 1 {
		t.Errorf("early action: predicate queries %d, want one with both witnesses", queries)
	}
}
