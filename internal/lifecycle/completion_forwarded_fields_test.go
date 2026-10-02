package lifecycle

import (
	"bytes"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestCompletionForwardedFields(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+`
 type box struct{first,second *resource}
 func acquire()*resource{return new(resource)}
 func closeOne(p *resource){p.Close()}
 func first(b *box){closeOne(b.first)}
 func earlier(b,c box){p:=b.first;b=c;closeOne(p)}
 func replacement(b,c box){b=c;closeOne(b.first)}
 func ambiguous(b,c box,flag bool){if flag{b=c};closeOne(b.first)}
 func runFirst(q *resource){p:=acquire();first(&box{p,q})}
 func runSibling(q *resource){p:=acquire();first(&box{q,p})}
 func runEarlier(q *resource){p:=acquire();earlier(box{p,q},box{q,q})}
 func runEarlierSibling(q *resource){p:=acquire();earlier(box{q,p},box{q,q})}
 func runReplacement(q *resource){p:=acquire();replacement(box{p,q},box{q,q})}
 func runAmbiguous(q *resource,flag bool){p:=acquire();ambiguous(box{p,q},box{q,q},flag)}
 `)
	for _, test := range []struct {
		name      string
		completes bool
	}{
		{"runFirst", true},
		{"runSibling", false},
		{"runEarlier", true},
		{"runEarlierSibling", false},
		{"runReplacement", false},
		{"runAmbiguous", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(test.name))
			if len(calls) != 2 || ssaflow.CallName(calls[0].Common()) != "acquire" {
				t.Fatal("expected acquisition followed by cleanup helper")
			}
			var ir bytes.Buffer
			if _, err := calls[1].Common().StaticCallee().WriteTo(&ir); err != nil {
				t.Fatal(err)
			}
			t.Log(ir.String())
			request := CompletionRequest{
				Instruction: calls[1], Target: calls[0], Methods: []string{"Close"},
				Budget: ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget),
			}
			proof := ProveCompletion(request)
			if proof.Proven() != test.completes {
				t.Fatalf("completion=%+v, want %v", proof, test.completes)
			}
			if test.completes {
				if !proof.PathKnown || proof.Path != "" {
					t.Fatalf("exact field target lost path: %+v", proof)
				}
				assertCompletionSpillCutoff(t, request)
			}
		})
	}
}
