package cancellationownership

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

func TestCancellationResultGuardDiscoveryCutoff(t *testing.T) {
	source := `package cancelguards
 import "context"
 var sink int
 func lost()(err error){_,cancel:=context.WithCancel(context.Background());defer func(){if err!=nil{cancel()}}();return nil}
 func released()(err error){_,cancel:=context.WithCancel(context.Background());defer func(){if err==nil{cancel()}}();return nil}
 func large()(err error){_,cancel:=context.WithCancel(context.Background());defer func(){if err!=nil{cancel()}}();n:=0;` +
		strings.Repeat("n++;", cancellationCompletionBudget+1) + `sink=n;return nil}`
	pkg := ssaflowtest.BuildPackage(t, "cancelguards", source)
	for _, test := range []struct {
		name string
		want CancellationOutcome
	}{
		{"lost", CancellationLost}, {"released", CancellationReleased}, {"large", CancellationUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			for _, value := range ssaflow.InstructionsOf[*ssa.Extract](fn) {
				if value.Index != 1 {
					continue
				}
				call, ok := value.Tuple.(*ssa.Call)
				if !ok {
					continue
				}
				got := proveCancellation(call, value, analysisTrace.Probe{}, nil, nil)
				if got.Outcome != test.want {
					t.Fatalf("cancellation proof = %+v, want %v", got, test.want)
				}
				return
			}
			t.Fatal("missing cancellation")
		})
	}
}

func TestCancellationResultGuardReturnCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cancelreturns", `package cancelreturns
 import "context"
 func skipped()(err error){_,cancel:=context.WithCancel(context.Background());defer func(){if err!=nil{cancel()}}();return nil}
 func released()(err error){_,cancel:=context.WithCancel(context.Background());defer func(){if err==nil{cancel()}}();return nil}
 `)
	for _, name := range []string{"skipped", "released"} {
		fn := pkg.Func(name)
		var cancel ssa.Value
		for _, extract := range ssaflow.InstructionsOf[*ssa.Extract](fn) {
			if extract.Index == 1 {
				cancel = extract
				break
			}
		}
		if cancel == nil {
			t.Fatal("missing cancel")
		}
		query := &cancellationClassifier{cancel: cancel}
		guards := lifecycle.ProveResultGuards(fn, lifecycle.CompletionRequest{Target: cancel, InvokeTarget: true})
		if !guards.Proven() || len(guards.Guards) != 1 {
			t.Fatalf("discovery=%+v", guards)
		}
		query.guards = guards.Guards
		for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
			if !ssaflow.InstructionDominates(query.guards[0].Defer, returned) {
				continue
			}
			query.pool = ssaflow.NewSearchBudget(0)
			label, ok := query.resultGuardedReturn(returned)
			if !ok || label.action != cancellationActionUnknown {
				t.Fatalf("cut=%+v/%v", label, ok)
			}
			query.pool = ssaflow.NewSearchBudget(cancellationPoolBudget)
			label, ok = query.resultGuardedReturn(returned)
			if name == "released" && (!ok || label.action != cancellationActionRelease) {
				t.Fatalf("fresh release=%+v/%v", label, ok)
			}
			if name == "skipped" && ok {
				t.Fatalf("fresh skip=%+v/%v", label, ok)
			}
		}
	}
}
