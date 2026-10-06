package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestResourcePresenceAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "presencebudget", `package presencebudget
type resource struct{}
func direct(value *resource) { if value != nil { println(value) } }
func reversed(value *resource) { if nil == value { println(value) } }
func asserted(value *resource) { var boxed any = value; if _, ok := boxed.(*resource); ok { println(value) } }
func unrelated(value, other *resource) { if other != nil { println(value) } }
func incompatible(value *resource) { var boxed any = value; if _, ok := boxed.(*int); ok { println(value) } }
`)
	for _, test := range []struct {
		name    string
		known   bool
		present bool
	}{{"direct", true, true}, {"reversed", true, false}, {"asserted", true, true}, {"unrelated", false, false}, {"incompatible", false, false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
			block := branch.Block()
			t.Log(branch.String())
			for arm, successor := range block.Succs {
				wantPresent := test.present == (arm == 0)
				completed := false
				for limit := 0; limit <= proofs.SummaryBudget; limit++ {
					pool := proofs.NewSearchBudget(limit)
					budget := pool.Within(proofs.SummaryBudget)
					got := proveResourcePresenceBranch(block, nil, successor, fn.Params[0], budget)
					if resourceFlowExhausted(budget) {
						if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
							t.Fatalf("allowance %d admitted interrupted presence: %+v", limit, got)
						}
						continue
					}
					if got.Proven() != test.known || test.known && got.Present != wantPresent {
						t.Fatalf("completed arm %d presence = %+v", arm, got)
					}
					completed = true
					break
				}
				if !completed {
					t.Fatal("presence proof never completed")
				}
			}
		})
	}
}

func TestResourcePresenceUsesIncomingPath(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "presencepath", `package presencepath
type resource struct{}
func guarded(value *resource, flag bool) {
	condition := flag && value != nil
	if condition { println(value) }
}
`)
	fn := pkg.Func("guarded")
	var branch *ssa.If
	for _, candidate := range ssaflow.InstructionsOf[*ssa.If](fn) {
		if _, ok := candidate.Cond.(*ssa.Phi); ok {
			branch = candidate
		}
	}
	if branch == nil {
		t.Fatal("fixture lacks saved short-circuit phi")
	}
	block := branch.Block()
	for _, predecessor := range append([]*ssa.BasicBlock{nil, {}}, block.Preds...) {
		incoming := ssapath.BranchValueWithin(branch.Cond, block, predecessor, nil)
		_, known := incoming.(*ssa.BinOp)
		for arm, successor := range block.Succs {
			got := proveResourcePresenceBranch(block, predecessor, successor, fn.Params[0], proofs.NewSearchBudget(proofs.SummaryBudget))
			if got.Proven() != known || known && got.Present != (arm == 0) {
				t.Fatalf("incoming %v, arm %d presence = %+v", incoming, arm, got)
			}
		}
	}
}
