package summaries

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/resultfacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
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
	for index, want := range []ssaflow.Outcome{
		ssaflow.OutcomeTrue, ssaflow.OutcomeFalse, ssaflow.OutcomeNil, ssaflow.OutcomeNonNil,
		ssaflow.OutcomeNonNil, ssaflow.OutcomeAny, ssaflow.OutcomeAny,
	} {
		got, known := provider.OutcomeOf(results[index], proofs.NewSearchBudget(2000))
		if got != want || known != (want != ssaflow.OutcomeAny) {
			t.Errorf("result %d: outcome (%v,%v), want %v", index, got, known, want)
		}
	}
	for _, knowledge := range []*Provider{nil, Select(Requirements{}).Provider(pass), Select(Requirements{Results: true}).Provider(nil)} {
		for name, want := range map[string]ssaflow.Outcome{
			"yes": ssaflow.OutcomeTrue, "no": ssaflow.OutcomeFalse, "empty": ssaflow.OutcomeNil,
			"fresh": ssaflow.OutcomeNonNil, "boxed": ssaflow.OutcomeNonNil,
		} {
			value := ssaflow.InstructionsOf[*ssa.Return](pkg.Func(name))[0].Results[0]
			got, known := knowledge.OutcomeOf(value, nil)
			if !known || got != want {
				t.Errorf("%s: unavailable component lost literal/construction outcome", name)
			}
		}
		if got, known := knowledge.OutcomeOf(results[0], nil); known || got != ssaflow.OutcomeAny {
			t.Error("unavailable component invented a call outcome")
		}
	}
	budget := proofs.NewSearchBudget(1)
	budget.Spend()
	if got, known := provider.OutcomeOf(results[0], budget); known || got != ssaflow.OutcomeAny || !budget.Exhausted() {
		t.Error("exhausted summary query retained a call guarantee")
	}
}
