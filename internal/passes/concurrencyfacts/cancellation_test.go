package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func cancellationPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "cancellation", `package cancellation
import "context"
func stop(cancel context.CancelFunc) { cancel() }
func stopAgain(cancel context.CancelFunc) { stop(cancel) }
func wait(ctx context.Context) { <-ctx.Done() }
func worker(ctx context.Context, out chan int) {
 select { case out <- 1: case <-ctx.Done(): }
}
func root() {
 ctx, cancel := context.WithCancel(context.Background())
 go wait(ctx)
 stopAgain(cancel)
}
func choice() {
 ctx, cancel := context.WithCancel(context.Background())
 out := make(chan int)
 go worker(ctx, out)
 cancel()
}
func deferred() {
 ctx, cancel := context.WithCancel(context.Background())
 defer cancel()
 go wait(ctx)
}
func captured() {
 ctx, cancel := context.WithCancel(context.Background())
 go func() { <-ctx.Done() }()
 cancel()
}
func distinct() {
 first, cancelFirst := context.WithCancel(context.Background())
 second, cancelSecond := context.WithCancel(context.Background())
 go wait(first)
 go wait(second)
 cancelSecond()
 cancelFirst()
}
func cause() {
 ctx, cancel := context.WithCancelCause(context.TODO())
 go wait(ctx)
 cancel(nil)
}
func unknown(ctx context.Context, cancel context.CancelFunc) { go wait(ctx); stop(cancel) }
func parentUnknown(parent context.Context) {
 ctx, cancel := context.WithCancel(parent)
 go wait(ctx)
 cancel()
}
func nested() {
 parent, first := context.WithCancel(context.Background())
 ctx, second := context.WithCancel(parent)
 go wait(ctx)
 first(); second()
}
type custom struct { context.Context }
func (custom) Done() <-chan struct{} { select {} }
func customContext() { go wait(custom{context.Background()}) }
func customCancel() { stop(context.CancelFunc(func() { select {} })) }
func without() { ctx := context.WithoutCancel(context.Background()); go wait(ctx) }
func timed() { ctx, cancel := context.WithTimeout(context.Background(), 1); go wait(ctx); cancel() }
func after() {
 ctx, cancel := context.WithCancel(context.Background())
 context.AfterFunc(ctx, func() {})
 cancel()
}
func replaced() {
 ctx, cancel := context.WithCancel(context.Background())
 go func() { <-ctx.Done() }()
 ctx = context.Background()
 cancel()
}
`)
}

func TestCancellationUnknownBoundaries(t *testing.T) {
	pkg := cancellationPackage(t)
	for _, name := range []string{"unknown", "parentUnknown", "nested", "customContext", "customCancel", "without", "timed", "after", "replaced"} {
		result := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(2000))
		if result.Complete() || result.AlternativesComplete {
			t.Errorf("%s = %+v, want unknown", name, result)
		}
	}
}

func TestCancellationBindsHelpersAndCaptures(t *testing.T) {
	pkg := cancellationPackage(t)
	for _, name := range []string{"root", "deferred", "captured", "cause"} {
		result := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(2000))
		if !result.Complete() || len(result.Operations) != 1 || len(result.Workers) != 1 || len(result.Workers[0].Operations) != 1 {
			t.Errorf("%s = %+v, want cancel and one receiving worker", name, result)
			continue
		}
		cancel, receive := result.Operations[0], result.Workers[0].Operations[0]
		if cancel.Kind != Cancel || receive.Kind != Receive || cancel.Resource != receive.Resource || !cancel.Resource.Cancellation {
			t.Errorf("%s cancel=%+v receive=%+v", name, cancel, receive)
		}
	}
}

func TestCancellationSelectRetainsEscape(t *testing.T) {
	result := NewEngine().Root(cancellationPackage(t).Func("choice"), ssaflow.NewSearchBudget(2000))
	if result.Complete() || !result.AlternativesComplete || !result.CancellationBound() || len(result.Workers) != 1 ||
		len(result.Workers[0].Alternatives) != 2 || len(result.Operations) != 1 {
		t.Fatalf("select = %+v", result)
	}
	arm := result.Workers[0].Alternatives[1]
	if len(arm) != 1 || arm[0].Kind != Receive || arm[0].Resource != result.Operations[0].Resource {
		t.Errorf("cancellation arm = %+v", arm)
	}
}

func TestDistinctCancellationIdentities(t *testing.T) {
	result := NewEngine().Root(cancellationPackage(t).Func("distinct"), ssaflow.NewSearchBudget(2000))
	if !result.Complete() || len(result.Workers) != 2 || len(result.Operations) != 2 {
		t.Fatalf("distinct = %+v", result)
	}
	first, second := result.Workers[0].Operations[0].Resource, result.Workers[1].Operations[0].Resource
	if first == second || result.Operations[0].Resource != second || result.Operations[1].Resource != first {
		t.Errorf("two contexts conflated: %+v", result)
	}
}
