package summaries

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/passes/resultfacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestPairedNilnessPathAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "pairedguard", `package pairedguard
type box struct{}
type failure struct{}
func (*failure) Error() string { return "failure" }
func Open(ok bool) (*box, error) { if ok { return &box{}, nil }; return nil, &failure{} }
func identity(err error) error { return err }
func guarded(ok bool) *box {
 b, err := Open(ok)
`+strings.Repeat("err = identity(err)\n", 64)+`if err != nil { return nil }
 if b == nil { return nil }
 return b
}
`)
	fn := pkg.Func("guarded")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	value := ssacall.CallResult(call, 0)
	branches := ssaflow.InstructionsOf[*ssa.If](fn)
	if len(branches) != 2 || len(ssaflow.InstructionsOf[*ssa.Call](fn)) != 65 {
		t.Fatal("actual SSA must retain both guards and 64 error provenance calls")
	}
	block := branches[1].Block()
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}}
	provider := Select(Requirements{Results: true}).Provider(pass)
	// Warm the callee alone so the small allowance isolates caller metadata
	// and path evidence, rather than rediscovering the callee's result cases.
	if _, availability := provider.ForFunction(pkg.Func("Open")).Results(proofs.NewSearchBudget(proofs.SummaryBudget)); availability != Available {
		t.Fatal("fixture must publish complete conditional result cases")
	}
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	cut := pool.Within(16)
	if _, known := provider.pairedNilness(value, block, cut); known || !cut.Exhausted() || pool.Exhausted() {
		t.Fatal("paired guard path bypassed its allowance or exhausted the outer pool")
	}
	complete := false
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		isNil, known := provider.pairedNilness(value, block, budget)
		if budget.Exhausted() {
			if known {
				t.Fatal("interrupted path establishes paired nilness")
			}
			continue
		}
		if !known || isNil {
			t.Fatalf("fresh paired result = nil %v, known %v", isNil, known)
		}
		complete = true
		break
	}
	if !complete {
		t.Fatal("fresh paired result never completed after cutoff")
	}
	if successors := provider.FeasibleSuccessors(block, nil, proofs.NewSearchBudget(16)); len(successors) != 2 {
		t.Fatal("interrupted paired evidence pruned a feasible successor")
	}
	if successors := provider.FeasibleSuccessors(block, nil, proofs.NewSearchBudget(proofs.SummaryBudget)); len(successors) != 1 ||
		successors[0] != block.Succs[1] {
		t.Fatal("fresh paired evidence lost the nonnil result branch")
	}
}
