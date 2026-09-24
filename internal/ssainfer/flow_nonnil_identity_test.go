package ssainfer

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestNonNilAssumptionRequiresExactIdentity(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
func direct(value *int) {
 println("start")
 if value != nil { println("owned") }
}
func replaced(value *int, drop bool) {
 println("start")
 alias := value
 if drop { alias = nil }
 if alias != nil { println("owned") }
}
`)
	for _, test := range []struct {
		name string
		lost bool
	}{
		{"direct", false},
		{"replaced", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			owns := func(instruction ssa.Instruction) bool { return instruction == calls[1] }
			if got := ssaflow.UnownedReturnAssumingNonNil(calls[0], function.Params[0], owns, nil); got != test.lost {
				t.Fatalf("unowned return = %t, want %t", got, test.lost)
			}
		})
	}
}
