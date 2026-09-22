package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestHelperSummaryBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
func receive(ch chan struct{}) { <-ch }
func forward(ch chan struct{}) { receive(ch) }
func ignore(ch chan struct{}) {}
func recursive(ch chan struct{}) { recursive(ch); receive(ch) }
`)
	for _, test := range []struct {
		name  string
		limit int
		want  ownershipAction
	}{
		{"forward", 5, actionJoin}, // Enters receive, then runs out before its return.
		{"ignore", 1, actionNone},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			search := newHelperSearch()
			search.budget = ssaflow.NewSearchBudget(test.limit)
			if got := search.use(function, function.Params[0], trackedSignal); got != actionUnknown {
				t.Fatalf("shortened answer = %v, want unknown (neither join nor absence)", got)
			}
			search.budget = ssaflow.NewSearchBudget(helperUseBudget)
			if got := search.use(function, function.Params[0], trackedSignal); got != test.want {
				t.Errorf("fresh budget = %v, want %v; incomplete answer must not be cached", got, test.want)
			}
		})
	}
	function := pkg.Func("recursive")
	search := newHelperSearch()
	search.budget = ssaflow.NewSearchBudget(1)
	if got := search.use(function, function.Params[0], trackedSignal); got != actionUnknown || !search.budget.Exhausted() {
		t.Errorf("recursive query = %v, exhausted=%v", got, search.budget.Exhausted())
	}
}
