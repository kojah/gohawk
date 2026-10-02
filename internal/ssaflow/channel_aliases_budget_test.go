package ssaflow_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const channelCensusFixture = `package channelcensus
 var kept chan int
 func send(out chan<- int){out<-1}
 func captured(){done:=make(chan int);go func(){close(done)}()}
 func passed(){done:=make(chan int);go send(done);<-done}
 func escaped(){done:=make(chan int);kept=done}
 func oldSnapshot(){var done chan int;before:=done;done=make(chan int);go func(){close(done)}();println(before)}
 func lateCapture(){var done chan int;go func(){close(done)}();done=make(chan int)}
 func conditional(choose bool){var done chan int;if choose{done=make(chan int)};go func(){close(done)}()}
 func nested(){done:=make(chan int);go func(){func(){close(done)}()}()}
 func repeated(n int){var done chan int;for i:=0;i<n;i++{before:=done;done=make(chan int);go func(){close(done)}();println(before)}}
`

func TestChannelCensusAllowanceAndTiming(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "channelcensus", channelCensusFixture)
	for _, test := range []struct {
		name  string
		known bool
		uses  int
	}{
		{"captured", true, 1},
		{"passed", true, 2},
		{"escaped", true, 1},
		{"oldSnapshot", true, 1},
		{"lateCapture", false, 0},
		{"conditional", false, 0},
		{"nested", true, 1},
		{"repeated", false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			made := ssaflow.InstructionsOf[*ssa.MakeChan](fn)[0]
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			for _, body := range fn.AnonFuncs {
				if _, err := body.WriteTo(&dump); err != nil {
					t.Fatal(err)
				}
			}
			t.Log(dump.String())
			want := ssaflow.ProveChannelValuesWithin(made, nil)
			if want.Proven() != test.known || len(want.Uses) != test.uses {
				t.Fatalf("census=%+v", want)
			}
			if test.name == "oldSnapshot" && slices.Contains(want.Values, ssa.Value(ssaflow.InstructionsOf[*ssa.UnOp](fn)[0])) {
				t.Fatal("pre-store snapshot is not the newly made channel")
			}
			for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
				budget := ssaflow.NewSearchBudget(limit)
				got := ssaflow.ProveChannelValuesWithin(made, budget)
				if budget.Exhausted() {
					if got.Reason != ssaflow.EvidenceBudgetExhausted || got.Values != nil || got.Uses != nil {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("complete%d=%+v want%+v", limit, got, want)
				}
				return
			}
			t.Fatal("census never completed")
		})
	}
}

func TestChannelCensusPartialUseDiscarded(t *testing.T) {
	source := `package partialchannel
 func ignore(done chan int){}
 func subject(){done:=make(chan int);close(done);` + strings.Repeat("ignore(done);", ssaflow.QueryBudget+1) + `}
 `
	fn := ssaflowtest.BuildPackage(t, "partialchannel", source).Func("subject")
	made := ssaflow.InstructionsOf[*ssa.MakeChan](fn)[0]
	pool := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	child := pool.Within(ssaflow.QueryBudget)
	got := ssaflow.ProveChannelValuesWithin(made, child)
	if got.State != ssaflow.EvidenceUnknown || got.Values != nil || got.Uses != nil || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut=%+v", got)
	}
	fresh := ssaflow.ProveChannelValuesWithin(made, pool.Within(2*ssaflow.SummaryBudget))
	if !fresh.Proven() || len(fresh.Uses) != 1 {
		t.Fatalf("fresh=%+v", fresh)
	}
}
