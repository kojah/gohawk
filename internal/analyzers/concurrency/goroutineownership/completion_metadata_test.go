package goroutineownership

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCompletionCapturePrecedesArgumentMetadata(t *testing.T) {
	var parameters, arguments []string
	for index := range 64 {
		parameters = append(parameters, fmt.Sprintf("p%d int", index))
		arguments = append(arguments, "0")
	}
	pkg := ssaflowtest.BuildPackage(t, "bindingmetadata", "package bindingmetadata; type owner struct{n int}; func subject(){var group owner; go func("+
		strings.Join(parameters, ",")+"){group.n++}("+strings.Join(arguments, ",")+")}")
	function := pkg.Func("subject")
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	worker, closure := ssaflow.DirectCallee(spawn.Common())
	owner := closure.Bindings[0]
	// The capture is exact without consulting any of the unrelated parameters.
	// A fixed allowance must not be spent preparing their metadata first.
	budget := ssaflow.NewSearchBudget(32)
	if got := completionValueAtCall(spawn, worker, closure, worker.FreeVars[0], budget); got != owner || budget.Exhausted() {
		t.Fatalf("capture binding = %v, exhausted=%v; want %v", got, budget.Exhausted(), owner)
	}
	for limit := range 32 {
		budget := ssaflow.NewSearchBudget(limit)
		got := completionValueAtCall(spawn, worker, closure, worker.FreeVars[0], budget)
		if budget.Exhausted() && got != nil {
			t.Fatalf("limit %d published capture after cutoff: %v", limit, got)
		}
		if got != nil && got != owner {
			t.Fatalf("limit %d selected another binding: %v", limit, got)
		}
	}
}
