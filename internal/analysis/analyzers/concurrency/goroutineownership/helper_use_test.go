package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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

func TestHelperRecursiveCutDoesNotPoisonRetry(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
func receive(ch chan struct{}) { <-ch }
`)
	function := pkg.Func("receive")
	search := newHelperSearch()
	var shortened ownershipAction
	if !search.memo.WithFunction(function, func() {
		shortened = search.use(function, function.Params[0], trackedSignal)
	}) {
		t.Fatal("could not enter helper")
	}
	if shortened != actionUnknown {
		t.Errorf("recursive cut = %v, want unknown", shortened)
	}
	if got := search.use(function, function.Params[0], trackedSignal); got != actionJoin {
		t.Errorf("fresh call path = %v, want join; recursive answer must not be cached", got)
	}
}
