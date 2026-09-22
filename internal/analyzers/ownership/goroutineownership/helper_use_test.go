package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestHelperSummaryKeepsTargetAndKind(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
func receive(first, second chan struct{}) { <-first }
func forward(first, second chan struct{}) { receive(first, second) }
`)
	search := newHelperSearch()
	for _, name := range []string{"forward", "receive"} {
		function := pkg.Func(name)
		for _, test := range []struct {
			parameter int
			kind      trackedKind
			want      ownershipAction
		}{
			{0, trackedSignal, actionJoin},
			{1, trackedSignal, actionNone},
			{0, trackedOwner, actionNone},
			{0, trackedSignal, actionJoin},
		} {
			if got := search.use(function, function.Params[test.parameter], test.kind); got != test.want {
				t.Errorf("%s parameter %d kind %d: got %v, want %v", name, test.parameter, test.kind, got, test.want)
			}
		}
	}
}
