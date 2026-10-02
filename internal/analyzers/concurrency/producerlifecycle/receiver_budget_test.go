package producerlifecycle

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestHelperReceiverCutoffKeepsUnknown(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receiverbudget", `package receiverbudget
func receive(ch chan int) { <-ch }
func subject(ch chan int) { receive(ch) }
func receiveTwice(ch chan int) { <-ch; <-ch }
func subjectTwice(ch chan int) { receiveTwice(ch) }
func worker(ch chan int, opaque func()) { opaque(); ch <- 1 }
func launched(ch chan int, opaque func()) { go worker(ch, opaque) }
`)
	for name, count := range map[string]int{"subject": 1, "subjectTwice": 2, "launched": 0} {
		fn := pkg.Func(name)
		var dump strings.Builder
		if _, err := fn.WriteTo(&dump); err != nil {
			t.Fatal(err)
		}
		t.Log(dump.String())
		call := fn.Blocks[0].Instrs[0].(ssa.CallInstruction)
		checkHelperReceiverBudgets(t, call, fn.Params[0], count)
	}
}

func checkHelperReceiverBudgets(t *testing.T, call ssa.CallInstruction, channel ssa.Value, count int) {
	t.Helper()
	for _, warm := range []bool{false, true} {
		complete := false
		for limit := range 256 {
			engine := concurrencyfacts.NewEngine()
			if warm {
				summary := engine.AtCall(call, nil)
				if summary.Complete() != (count > 0) {
					t.Fatal("unexpected warm summary availability")
				}
			}
			budget := ssaflow.NewSearchBudget(limit)
			proof := helperReceives(call, channel, nil, engine, budget)
			if budget.Exhausted() {
				if !proof.unknown || proof.count != 0 {
					t.Fatalf("warm=%v cutoff=%d supplied a count: %+v", warm, limit, proof)
				}
				if proof.reason != reasonReceiverBudgetExhausted {
					t.Errorf("warm=%v cutoff=%d reason=%s", warm, limit, proof.reason.String())
				}
				continue
			}
			if proof.unknown || proof.count != count {
				t.Fatalf("warm=%v complete=%d: %+v", warm, limit, proof)
			}
			complete = true
			break
		}
		if !complete {
			t.Fatalf("warm=%v never completed", warm)
		}
	}
}
