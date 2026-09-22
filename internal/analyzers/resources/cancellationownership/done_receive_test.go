package cancellationownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDoneReceiveOutcome(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "done", `package done
import "context"
func received(parent context.Context) {
 ctx, cancel := context.WithCancel(parent)
 _ = cancel
 <-ctx.Done()
}
func selected(parent context.Context, work <-chan bool) {
 ctx, cancel := context.WithCancel(parent)
 select { case <-ctx.Done(): case <-work: cancel() }
}
func alternate(parent context.Context, work <-chan bool) {
 ctx, cancel := context.WithCancel(parent)
 _ = cancel
 select { case <-ctx.Done(): return; case <-work: return }
}
`)
	for _, test := range []struct {
		name string
		want CancellationOutcome
	}{
		{"received", CancellationUnknown},
		{"selected", CancellationUnknown},
		{"alternate", CancellationLost},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range ssaflow.InstructionsOf[*ssa.Extract](pkg.Func(test.name)) {
				if value.Index != 1 {
					continue
				}
				call, ok := value.Tuple.(*ssa.Call)
				if !ok {
					continue
				}
				if proof := proveCancellation(call, value, nil, nil); proof.Outcome != test.want {
					t.Fatalf("proof = %+v, want outcome %v", proof, test.want)
				}
				return
			}
			t.Fatal("missing cancellation acquisition")
		})
	}
}
