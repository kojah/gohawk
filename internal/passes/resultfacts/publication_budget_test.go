package resultfacts

import (
	"go/types"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestResultPublicationRejectsProofCutoff(t *testing.T) {
	source := `package publication
import "os"
func marker() {}
func Die() { os.Exit(0) }
func Heavy() { ` + strings.Repeat("marker();", 600) + `os.Exit(0) }
func Identity(p *int) *int { return p }
func HeavyIdentity(p *int) *int { ` + strings.Repeat("*p = 1;", 550) + `return p }
type failure struct{}
func (*failure) Error() string { return "failure" }
func Boxed() any { var err error = (*failure)(nil); return err }
func HeavyFold(flag bool) error { var err error = (*failure)(nil); ` + strings.Repeat("if flag {err = &failure{}};", 200) + `return err }
func Predicate(err error) bool { return err != nil }
func HeavyPredicate(err error, sink *int) bool { ` + strings.Repeat("if err != nil { *sink = 1 };", 150) + `return err != nil }
`
	directory, cleanup, err := analysistest.WriteFiles(map[string]string{"publication/source.go": source})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	probe := *Analyzer
	probe.Name = "terminationbudgetfacts"
	probe.Run = func(pass *analysis.Pass) (any, error) {
		if pass.Pkg.Path() != "publication" {
			return run(pass)
		}
		publishedFacts := make(map[string]Fact)
		// Observe the domain's actual publication boundary without forwarding
		// facts to analysistest's golden matcher; wire-format tests are separate.
		pass.ExportObjectFact = func(object types.Object, published analysis.Fact) {
			if strings.HasPrefix(object.Name(), "Heavy") {
				t.Errorf("interrupted %s inference must not publish a fact", object.Name())
			}
			publishedFacts[object.Name()] = published.(*publishedFact).Value()
		}
		result, runErr := run(pass)
		if runErr != nil {
			return nil, runErr
		}
		checkPublishedResultControls(t, publishedFacts)
		functions, sourceErr := ssaflow.SourceSSAFunctions(pass)
		if sourceErr != nil {
			return nil, sourceErr
		}
		checkFreshResultProofs(t, result.(*Engine), functions)
		return result, nil
	}
	analysistest.Run(t, directory, &probe, "publication")
}

func checkPublishedResultControls(t *testing.T, facts map[string]Fact) {
	t.Helper()
	if len(facts["Predicate"].Cases) != 1 {
		t.Error("fresh predicate must publish its conditional case")
	}
	if len(facts["Identity"].Returned) != 1 {
		t.Error("fresh identity control must publish its relation")
	}
	if len(facts["Boxed"].Results) != 1 || facts["Boxed"].Results[0] != AlwaysNonNil {
		t.Error("fresh boxed result must retain its nonnil guarantee")
	}
	if !facts["Die"].NeverReturns {
		t.Error("fresh direct-exit control must publish termination")
	}
}

func checkFreshResultProofs(t *testing.T, engine *Engine, functions []*ssa.Function) {
	t.Helper()
	for _, function := range functions {
		if function.Name() == "HeavyFold" {
			got := engine.Function(function, proofs.NewSearchBudget(4*proofs.SummaryBudget))
			if !got.Available || got.Result(0) != AlwaysNonNil {
				t.Error("publication cutoff poisoned fresh folded result inference")
			}
		}
		if function.Name() == "HeavyPredicate" {
			got := engine.Function(function, proofs.NewSearchBudget(4*proofs.SummaryBudget))
			if !got.Available || !got.Implies(ssaflow.ParameterNil(0), 0, ssaflow.OutcomeFalse) {
				t.Error("publication cutoff poisoned fresh conditional inference")
			}
		}
		if function.Name() == "HeavyIdentity" {
			got := engine.Function(function, proofs.NewSearchBudget(4*proofs.SummaryBudget))
			parameter, proven := got.ReturnedParameter(0)
			if !got.Available || !proven || parameter != 0 {
				t.Error("publication cutoff poisoned fresh returned-parameter inference")
			}
		}
		if function.Name() == "Heavy" {
			got := engine.Function(function, proofs.NewSearchBudget(2*proofs.SummaryBudget))
			if !got.Available || !got.NeverReturns() {
				t.Error("publication cutoff must not poison a fresh complete summary")
			}
		}
	}
}
