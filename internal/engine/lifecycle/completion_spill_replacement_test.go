package lifecycle

import (
	"bytes"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A field target requires the original aggregate at the receiver's read.
// Saved values and agreeing writes preserve that identity; derivation alone
// cannot credit replacement contents or ambiguous branch writes.
func TestCompletionSpillReplacement(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 type box struct {first *resource}
 func acquire()*resource{return new(resource)}
 func (b box)Close(){b.first.Close()}
 func replacement(b,c box){b=c;b.first.Close()}
 func earlier(b,c box){p:=b.first;b=c;p.Close()}
 func wrapped(b,c box){p:=b.first;b=c;var closer interface{Close()}=p;closer.Close()}
 func restored(b,c box){original:=b;b=c;p:=b.first;b=original;p.Close()}
 func ambiguous(b,c box,flag bool){if flag{b=c};b.first.Close()}
 func agreeing(b,c box){original:=b;if c.first!=nil{b=original};b.first.Close()}
 func whole(b,c box){b.Close()}
 func dynamic(files [2]*resource){for _,r:=range files{r.Close()}}
 func runReplacement(q *resource){p:=acquire();replacement(box{p},box{q})}
 func runEarlier(q *resource){p:=acquire();earlier(box{p},box{q})}
 func runWrapped(q *resource){p:=acquire();wrapped(box{p},box{q})}
 func runRestored(q *resource){p:=acquire();restored(box{p},box{q})}
 func runAmbiguous(q *resource,flag bool){p:=acquire();ambiguous(box{p},box{q},flag)}
 func runAgreeing(q *resource){p:=acquire();agreeing(box{p},box{q})}
 func runWhole(q *resource){p:=acquire();whole(box{p},box{q})}
 func runDynamic(q *resource){p:=acquire();dynamic([2]*resource{p,q})}
 `)
	for _, test := range []struct {
		name      string
		completes bool
	}{
		{"runReplacement", false},
		{"runEarlier", true},
		{"runWrapped", true},
		{"runRestored", false},
		{"runAmbiguous", false},
		{"runAgreeing", true},
		{"runWhole", true},
		{"runDynamic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](fn)
			if len(calls) != 2 || ssaflow.CallName(calls[0].Common()) != "acquire" {
				t.Fatal("expected acquisition followed by cleanup helper")
			}
			var ir bytes.Buffer
			if _, err := calls[1].Common().StaticCallee().WriteTo(&ir); err != nil {
				t.Fatal(err)
			}
			t.Log(ir.String())
			request := CompletionRequest{Instruction: calls[1], Target: calls[0], Methods: []string{"Close"}}
			request.Budget = proofs.NewSearchBudget(10 * proofs.SummaryBudget)
			proof := ProveCompletion(request)
			if proof.Proven() != test.completes {
				t.Fatalf("completion=%+v, want %v", proof, test.completes)
			}
			if test.name == "runDynamic" && proof.Reason != proofs.EvidenceCompletionInCycle {
				t.Fatalf("dynamic cleanup lost loop uncertainty: %+v", proof)
			}
			if test.completes {
				assertCompletionSpillCutoff(t, request)
			}
		})
	}
}

func assertCompletionSpillCutoff(t *testing.T, request CompletionRequest) {
	t.Helper()
	sawCut := false
	for limit := 1; limit <= 10*proofs.SummaryBudget; limit += 10 {
		request.Budget = proofs.NewSearchBudget(limit)
		proof := ProveCompletion(request)
		if request.Budget.Exhausted() {
			sawCut = true
			if proof.Proven() || proof.PathKnown {
				t.Fatalf("cutoff %d published completion %+v", limit, proof)
			}
			continue
		}
		if !sawCut || !proof.Proven() {
			t.Fatalf("fresh allowance %d failed to recover: %+v", limit, proof)
		}
		return
	}
	t.Fatal("completion never recovered within allowance")
}
