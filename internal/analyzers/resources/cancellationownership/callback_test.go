package cancellationownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// Assert the proof outcome, not just silence: unknown and proven release both
// suppress a diagnostic, but only the latter establishes exact completion.
func TestCallbackCompletionOutcomes(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "callbacks", `package callbacks
import "context"
func invoke(fn context.CancelFunc) { fn() }
func forward(fn context.CancelFunc) { invoke(fn) }
func capture(fn context.CancelFunc) { func() { fn() }() }
func ignore(fn context.CancelFunc) {}
func second(fn, other context.CancelFunc) { other() }
func launch(fn context.CancelFunc) { go fn() }
func viaCallback(fn context.CancelFunc, callback func(context.CancelFunc)) { callback(fn) }
func direct() { _, cancel := context.WithCancel(context.Background()); invoke(cancel) }
func forwarded() { _, cancel := context.WithCancel(context.Background()); forward(cancel) }
func captured() { _, cancel := context.WithCancel(context.Background()); capture(cancel) }
func ignored() { _, cancel := context.WithCancel(context.Background()); ignore(cancel) }
func wrong() { _, cancel := context.WithCancel(context.Background()); second(cancel, func(){}) }
func conditional(yes bool) { _, cancel := context.WithCancel(context.Background()); if yes { forward(cancel) } }
func asynchronous() { _, cancel := context.WithCancel(context.Background()); go forward(cancel) }
func helperLaunch() { _, cancel := context.WithCancel(context.Background()); launch(cancel) }
func bound() { _, cancel := context.WithCancel(context.Background()); viaCallback(cancel, func(value context.CancelFunc) { value() }) }
`)
	for _, test := range []struct {
		name string
		want CancellationOutcome
	}{
		{"direct", CancellationReleased},
		{"forwarded", CancellationReleased},
		{"captured", CancellationReleased},
		{"ignored", CancellationLost},
		{"wrong", CancellationLost},
		{"conditional", CancellationLost},
		{"asynchronous", CancellationUnknown},
		{"helperLaunch", CancellationUnknown},
		{"bound", CancellationReleased},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cancel *ssa.Extract
			for _, value := range ssaflow.InstructionsOf[*ssa.Extract](pkg.Func(test.name)) {
				if value.Index == 1 {
					cancel = value
				}
			}
			if cancel == nil {
				t.Fatal("missing cancel acquisition")
			}
			call, ok := cancel.Tuple.(*ssa.Call)
			if !ok {
				t.Fatal("cancel did not come from a call")
			}
			if proof := proveCancellation(call, cancel); proof.Outcome != test.want {
				t.Fatalf("proof = %+v, want outcome %v", proof, test.want)
			}
		})
	}
}
