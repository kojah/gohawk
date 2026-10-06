package summaries

import (
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/resultfacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestUnconditionalValueOutcomes(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "outcomes", `package outcomes
type box struct{}
func yes() bool { return true }
func no() bool { return false }
func empty() *box { return nil }
func fresh() *box { return new(box) }
func boxed() any { var p *box; return p }
func unknown(flag bool) bool { return flag }
func opaque() bool
func caller(flag bool) (bool,bool,*box,*box,any,bool,bool) {
 return yes(),no(),empty(),fresh(),boxed(),unknown(flag),opaque()
}`)
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}}
	provider := Select(Requirements{Results: true}).Provider(pass)
	results := ssaflow.InstructionsOf[*ssa.Return](pkg.Func("caller"))[0].Results
	for index, want := range []ssacall.Outcome{
		ssacall.OutcomeTrue, ssacall.OutcomeFalse, ssacall.OutcomeNil, ssacall.OutcomeNonNil,
		ssacall.OutcomeNonNil, ssacall.OutcomeAny, ssacall.OutcomeAny,
	} {
		got, known := provider.OutcomeOf(results[index], proofs.NewSearchBudget(2000))
		if got != want || known != (want != ssacall.OutcomeAny) {
			t.Errorf("result %d: outcome (%v,%v), want %v", index, got, known, want)
		}
	}
	for _, knowledge := range []*Provider{nil, Select(Requirements{}).Provider(pass), Select(Requirements{Results: true}).Provider(nil)} {
		for name, want := range map[string]ssacall.Outcome{
			"yes": ssacall.OutcomeTrue, "no": ssacall.OutcomeFalse, "empty": ssacall.OutcomeNil,
			"fresh": ssacall.OutcomeNonNil, "boxed": ssacall.OutcomeNonNil,
		} {
			value := ssaflow.InstructionsOf[*ssa.Return](pkg.Func(name))[0].Results[0]
			got, known := knowledge.OutcomeOf(value, nil)
			if !known || got != want {
				t.Errorf("%s: unavailable component lost literal/construction outcome", name)
			}
		}
		if got, known := knowledge.OutcomeOf(results[0], nil); known || got != ssacall.OutcomeAny {
			t.Error("unavailable component invented a call outcome")
		}
	}
	budget := proofs.NewSearchBudget(1)
	budget.Spend()
	if got, known := provider.OutcomeOf(results[0], budget); known || got != ssacall.OutcomeAny || !budget.Exhausted() {
		t.Error("exhausted summary query retained a call guarantee")
	}
}
