package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCancellationSummaryIsNotJoin(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "canceljoin", `package canceljoin
import "context"
func stop(cancel context.CancelFunc) { cancel() }
func root() {
 _, cancel := context.WithCancel(context.Background())
 done := make(chan struct{})
 go func() { defer close(done) }()
 stop(cancel)
}
`)
	root := pkg.Func("root")
	channels := ssaflow.InstructionsOf[*ssa.MakeChan](root)
	if len(channels) != 1 {
		t.Fatal("missing completion channel")
	}
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](root) {
		if call.Common().StaticCallee() != pkg.Func("stop") {
			continue
		}
		engine := concurrencyfacts.NewEngine()
		summary := engine.AtCall(call, ssaflow.NewSearchBudget(2000))
		if !summary.Complete() || len(summary.Operations) != 1 || summary.Operations[0].Kind != concurrencyfacts.Cancel {
			t.Fatalf("cancellation not modeled: %+v", summary)
		}
		proof := proveSummaryJoin(engine, call, channels[0], trackedSignal, ssaflow.NewSearchBudget(2000))
		if proof.joined || proof.reason != "concurrency-summary-no-exact-join" {
			t.Errorf("cancel became join: %+v", proof)
		}
		return
	}
	t.Fatal("missing cancellation call")
}
