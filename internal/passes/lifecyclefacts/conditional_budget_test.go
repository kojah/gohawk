package lifecyclefacts

import (
	"go/types"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/analysis"
)

func TestConditionalCoverageCutCannotPublish(t *testing.T) {
	source := `package lifecyclefactstest
 type resource struct{}
 func(*resource)Close(){}
 func Small(p *resource,yes bool)bool{if yes{p.Close();return true};return false}
 func Large(p *resource,yes bool)bool{if yes{p.Close();` + strings.Repeat("println(1)\n", conditionalExportBudget+1) + `return true};return false}
 `
	pkg := buildLifecycleTestSSA(t, source)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	small := summarizeConditional(pass, pkg.Func("Small"))
	if len(small) == 0 {
		t.Fatal("small positive case was not inferred")
	}
	large := summarizeConditional(pass, pkg.Func("Large"))
	if len(large) != 0 {
		t.Fatalf("interrupted large body retained cases: %+v", large)
	}
	encoded := publish(Fact{Discharges: large})
	if got := encoded.Value().Discharges; len(got) != 0 {
		t.Fatalf("publication retained interrupted cases: %+v", got)
	}
	fn := pkg.Func("Small")
	condition := ssacall.CallCondition{Outcome: ssacall.OutcomeTrue}
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		_, ok := provenCasePath(fn, condition, lifecycle.CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}, Budget: budget, ExactTarget: true})
		if budget.Exhausted() {
			if ok {
				t.Fatalf("cut%d exposed a claim to publication", limit)
			}
			continue
		}
		if !ok {
			t.Fatalf("completed%d lost exact case", limit)
		}
		return
	}
	t.Fatal("case never completed")
}
