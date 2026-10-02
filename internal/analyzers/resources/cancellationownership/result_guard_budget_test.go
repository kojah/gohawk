package cancellationownership

import (
	"strings"
	"testing"

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
