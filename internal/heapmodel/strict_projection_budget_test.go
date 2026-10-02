package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStrictProjectionPathAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "strictprobe", `package strictprobe
 type node struct { child *node; value *int; items []*int }
 func observe(*int){}
 func field(p *node){observe(p.child.value)}
 func dynamic(p *node,i int){observe(p.items[i])}
 func other(p,q *node){observe(q.value)}
 `)
	for _, test := range []struct {
		name string
		want bool
	}{{"field", true}, {"dynamic", false}, {"other", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			value := strictObservedValue(t, fn)
			if ProveStrictProjectionPathWithin(value, fn.Params[0], nil).Proven() != test.want {
				t.Fatal("default projection differs")
			}
			if test.want {
				cut := ssaflow.NewSearchBudget(1)
				proof := ProveStrictProjectionPathWithin(value, fn.Params[0], cut)
				if proof.State != ssaflow.EvidenceUnknown || !cut.Exhausted() {
					t.Fatalf("path bypassed caller allowance: %+v", proof)
				}
			}
			for allowance := 1; allowance <= ssaflow.QueryBudget; allowance++ {
				budget := ssaflow.NewSearchBudget(allowance)
				proof := ProveStrictProjectionPathWithin(value, fn.Params[0], budget)
				if budget.Exhausted() {
					if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted {
						t.Fatalf("cut %d: %+v", allowance, proof)
					}
					continue
				}
				if proof.State == ssaflow.EvidenceUnknown || proof.Proven() != test.want {
					t.Fatalf("complete %d: %+v", allowance, proof)
				}
				return
			}
			t.Fatal("small projection never completed")
		})
	}
}

func TestStrictProjectionRetainsChildCap(t *testing.T) {
	source := `package strictprobe
 type node struct { child *node; value *int }
 func observe(*int){}
 func deep(p *node){observe(p.` + strings.Repeat("child.", ssaflow.QueryBudget+1) + `value)}
 func shallow(p *node){observe(p.value)}
 `
	pkg := ssaflowtest.BuildPackage(t, "strictprobe", source)
	fn := pkg.Func("deep")
	pool := ssaflow.NewSearchBudget(10 * ssaflow.QueryBudget)
	proof := ProveStrictProjectionPathWithin(strictObservedValue(t, fn), fn.Params[0], pool)
	if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted || pool.Exhausted() {
		t.Fatalf("child cutoff=%+v, parent exhausted=%v", proof, pool.Exhausted())
	}
	fn = pkg.Func("shallow")
	if proof := ProveStrictProjectionPathWithin(strictObservedValue(t, fn), fn.Params[0], pool); !proof.Proven() {
		t.Fatalf("fresh query=%+v", proof)
	}
}

func strictObservedValue(t *testing.T, fn *ssa.Function) ssa.Value {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) == "observe" {
			return call.Common().Args[0]
		}
	}
	t.Fatal("observation not found")
	return nil
}
