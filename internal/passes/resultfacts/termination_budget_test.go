package resultfacts

import (
	"go/types"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestTerminationReachabilityCutoffDoesNotCache(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", `package results
import "os"
func Die() { os.Exit(0) }
func Loop() { for {} }
func Returns() {}
func Deferred() { defer os.Exit(0) }
func Conditional(flag bool) { if flag { defer os.Exit(0) } }
`)
	for _, test := range []struct {
		name  string
		never bool
	}{{"Die", true}, {"Loop", true}, {"Returns", false}} {
		function := pkg.Func(test.name)
		// Lookup and the initial result census can finish, but no allowance
		// remains for the distinct reachability proof.
		limit := 1
		for _, block := range function.Blocks {
			limit += len(block.Instrs)
		}
		engine := NewEngine()
		cut := ssaflow.NewSearchBudget(limit)
		if got := engine.Function(function, cut); got.Available || got.NeverReturns() || !cut.Exhausted() {
			t.Fatalf("%s admitted an incomplete termination summary: %+v", test.name, got)
		}
		if got := engine.Function(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget)); !got.Available || got.NeverReturns() != test.never {
			t.Fatalf("%s cutoff poisoned fresh inference: %+v", test.name, got)
		}
	}
	for _, name := range []string{"Deferred", "Conditional"} {
		function := pkg.Func(name)
		if function.Recover == nil {
			t.Fatal("expected actual SSA recovery entry for deferred execution")
		}
		if got := NewEngine().Function(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget)); !got.Available || got.NeverReturns() {
			t.Fatal("recovery entry must retain the existing no-termination-claim boundary")
		}
	}
}

func TestResultPublicationRejectsProofCutoff(t *testing.T) {
	source := `package publication
import "os"
func marker() {}
func Die() { os.Exit(0) }
func Heavy() { ` + strings.Repeat("marker();", 600) + `os.Exit(0) }
func Identity(p *int) *int { return p }
func HeavyIdentity(p *int) *int { ` + strings.Repeat("*p = 1;", 550) + `return p }
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
		diePublished, identityPublished := false, false
		// Observe the domain's actual publication boundary without forwarding
		// facts to analysistest's golden matcher; wire-format tests are separate.
		pass.ExportObjectFact = func(object types.Object, published analysis.Fact) {
			if object.Name() == "Heavy" || object.Name() == "HeavyIdentity" {
				t.Errorf("interrupted %s inference must not publish a fact", object.Name())
			}
			if object.Name() == "Identity" {
				identityPublished = len(published.(*publishedFact).Value().Returned) == 1
			}
			if object.Name() == "Die" {
				diePublished = published.(*publishedFact).Value().NeverReturns
			}
		}
		result, runErr := run(pass)
		if runErr != nil {
			return nil, runErr
		}
		if !identityPublished {
			t.Error("fresh identity control must publish its relation")
		}
		if !diePublished {
			t.Error("fresh direct-exit control must publish termination")
		}
		functions, sourceErr := ssaflow.SourceSSAFunctions(pass)
		if sourceErr != nil {
			return nil, sourceErr
		}
		for _, function := range functions {
			if function.Name() == "HeavyIdentity" {
				got := result.(*Engine).Function(function, ssaflow.NewSearchBudget(4*ssaflow.SummaryBudget))
				parameter, proven := got.ReturnedParameter(0)
				if !got.Available || !proven || parameter != 0 {
					t.Error("publication cutoff poisoned fresh returned-parameter inference")
				}
			}
			if function.Name() == "Heavy" {
				got := result.(*Engine).Function(function, ssaflow.NewSearchBudget(2*ssaflow.SummaryBudget))
				if !got.Available || !got.NeverReturns() {
					t.Error("publication cutoff must not poison a fresh complete summary")
				}
			}
		}
		return result, nil
	}
	analysistest.Run(t, directory, &probe, "publication")
}
