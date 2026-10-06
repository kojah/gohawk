package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredHelperCallbackBindings(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest
func invoke(fn func()) { fn() }
func ignore(fn func()) {}
func generic[T any](fn func(), value T) { invoke(fn) }
func captured(fn func()) { defer func() { invoke(fn) }() }
func argument(fn func()) { defer func(value func()) { invoke(value) }(fn) }
func mixed(fn, other func()) { defer func(value func()) { ignore(other); invoke(value) }(fn) }
func wrongCapture(fn, other func()) { defer func() { invoke(other) }() }
func wrongArgument(fn, other func()) { defer func(value func()) { invoke(value) }(other) }
func ignored(fn func()) { defer func() { ignore(fn) }() }
func instantiated(fn func()) { defer generic(fn, 1) }
func ordinaryCall(fn func()) { func() { invoke(fn) }() }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		// This broad handoff helper does not prove invocation through a
		// captured cell's load. The classifier has a separate unknown
		// capture boundary; sharing bindings must not strengthen this answer.
		{"captured", false},
		{"argument", true},
		{"mixed", true},
		{"wrongCapture", false},
		{"wrongArgument", false},
		{"ignored", false},
		{"instantiated", true},
		{"ordinaryCall", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var instruction ssa.Instruction
			if deferred := ssaflow.InstructionsOf[*ssa.Defer](function); len(deferred) != 0 {
				instruction = deferred[0]
			} else {
				instruction = ssaflow.InstructionsOf[*ssa.Call](function)[0]
			}
			if got := DeferredClosureInvokesArgumentOnEveryReturn(instruction, function.Params[0]); got != test.want {
				t.Fatalf("deferred helper recognizes callback = %v, want %v", got, test.want)
			}
		})
	}
}
