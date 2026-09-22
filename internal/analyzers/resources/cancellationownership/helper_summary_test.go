package cancellationownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestCancellationSummaryKeepsParameter(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
var saved func()
func consume(first, second func()) { first(); saved = second }
func forward(first, second func()) { consume(first, second) }
`)
	search := newCancellationUse(nil)
	for _, name := range []string{"forward", "consume"} {
		function := pkg.Func(name)
		for _, parameter := range []int{0, 1, 0} {
			got := search.parameterResolved(function, function.Params[parameter])
			if got != (parameter == 0) {
				t.Errorf("%s parameter %d: resolved=%v, want %v", name, parameter, got, parameter == 0)
			}
		}
	}
}
