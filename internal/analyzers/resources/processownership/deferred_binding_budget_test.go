package processownership

import (
	"fmt"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredBindingMetadataAllowance(t *testing.T) {
	var params, args []string
	for index := range 64 {
		params = append(params, fmt.Sprintf("unused%d int", index))
		args = append(args, "0")
	}
	pkg := ssaflowtest.BuildPackage(t, "deferredbindings", "package deferredbindings;import \"os/exec\";func subject(cmd *exec.Cmd){marker:=0;defer func("+
		strings.Join(params, ",")+",actual *exec.Cmd){marker++;actual.Wait()}("+strings.Join(args, ",")+",cmd)}")
	fn := pkg.Func("subject")
	deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
	pool := proofs.NewSearchBudget(processPoolBudget)
	// Both implementations complete at 86 visits. This cutoff checks that
	// interrupting argument discovery stays unknown and does not poison a
	// fresh query sharing the candidate pool. The bounded iterator also avoids
	// eagerly preparing unvisited bindings, without changing per-visit cost.
	callback := fn.AnonFuncs[0]
	// Finish the body/capture census, then interrupt halfway through arguments.
	limit := len(ssaflow.InstructionsOf[ssa.Instruction](callback)) + len(callback.FreeVars) + 32
	child := pool.Within(limit)
	got := deferredClosureWaitsForCommand(deferred, fn.Params[0], child)
	if got != proofs.EvidenceUnknown || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("partial metadata=%v, exhausted=%v/%v", got, child.Exhausted(), pool.Exhausted())
	}
	fresh := pool.Within(processQueryBudget)
	if got := deferredClosureWaitsForCommand(deferred, fn.Params[0], fresh); got != proofs.EvidenceProven || fresh.Exhausted() {
		t.Fatalf("fresh metadata=%v, exhausted=%v", got, fresh.Exhausted())
	}
}
