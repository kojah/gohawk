package cancellationownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestCancellationSummaryBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
func consume(cancel func()) { cancel() }
func forward(cancel func()) { consume(cancel) }
func recursive(cancel func()) { recursive(cancel) }
`)
	function := pkg.Func("forward")
	search := newCancellationUse(nil)
	// Enter consume with the remaining budget; its return must still be charged.
	search.budget = ssaflow.NewSearchBudget(3)
	if search.parameterResolved(function, function.Params[0]) || !search.budget.Exhausted() {
		t.Fatal("nested work must exhaust the shared budget without proving resolved use")
	}
	search.budget = ssaflow.NewSearchBudget(cancellationCompletionBudget)
	if !search.parameterResolved(function, function.Params[0]) {
		t.Fatal("fresh budget failed to recover; shortened parent answer must not be cached")
	}
	function = pkg.Func("recursive")
	search = newCancellationUse(nil)
	search.budget = ssaflow.NewSearchBudget(1)
	if search.parameterResolved(function, function.Params[0]) || !search.budget.Exhausted() {
		t.Fatal("recursive work must remain bounded and unresolved")
	}
}
