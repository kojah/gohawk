package heapmodel

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestStrictParameterProjectionKeepsReadPath(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "strictparams", `package strictparams
 type box struct{first *int}
 func observe(interface{}){}
 func saved(b,c box){p:=b.first;b=c;observe(p)}
 func wrapped(b,c box){p:=b.first;b=c;var v interface{}=p;observe(v)}
 func replacement(b,c box){b=c;observe(b.first)}
 func ambiguous(b,c box,flag bool){if flag{b=c};observe(b.first)}
 func agreeing(b box,flag bool){original:=b;if flag{b=original};observe(b.first)}
 `)
	for _, test := range []struct {
		name   string
		proven bool
	}{
		{"saved", true},
		{"wrapped", true},
		{"replacement", false},
		{"ambiguous", false},
		{"agreeing", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			value := strictObservedValue(t, fn)
			sawCut := false
			for limit := 1; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				proof := ProveStrictProjectionPathWithin(value, fn.Params[0], budget)
				if budget.Exhausted() {
					sawCut = true
					if proof.State != proofs.EvidenceUnknown || proof.Path != nil {
						t.Fatalf("cutoff %d published %+v", limit, proof)
					}
					continue
				}
				if !sawCut || proof.Proven() != test.proven {
					t.Fatalf("fresh allowance %d: %+v", limit, proof)
				}
				if test.proven && !slices.Equal(proof.Path, []string{"field:0"}) || !test.proven && proof.Path != nil {
					t.Fatalf("projection lost exact path: %+v", proof)
				}
				return
			}
			t.Fatal("projection did not recover")
		})
	}
}
