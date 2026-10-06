package summaries

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestConcurrencyCallBindingParity(t *testing.T) {
	selection := Select(Requirements{Concurrency: true})
	consumer := &analysis.Analyzer{
		Name: "bindingparity", Doc: "assert broker and domain call binding parity", Requires: selection.Requires(),
		Run: func(pass *analysis.Pass) (any, error) {
			provider := selection.Provider(pass)
			engine, _ := provider.Concurrency()
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			checked := 0
			for _, function := range functions {
				if function.Name() != "Pair" && function.Name() != "LocalPair" && function.Name() != "Opaque" {
					continue
				}
				assertConcurrencyCaller(t, provider, engine, function)
				checked++
			}
			if checked != 3 {
				t.Errorf("checked %d callers, want 3", checked)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "brokerconsumer")
}

func assertPairBinding(t *testing.T, function *ssa.Function, call *ssa.Call, summary concurrencyfacts.Summary) {
	t.Helper()
	if !summary.Complete() || len(summary.Operations) != 4 {
		t.Errorf("%s: expected four complete effects, got %+v", function, summary)
		return
	}
	for index, kind := range []concurrencyfacts.Kind{concurrencyfacts.Lock, concurrencyfacts.Lock, concurrencyfacts.Unlock, concurrencyfacts.Unlock} {
		parameter := []int{0, 1, 1, 0}[index]
		operation := summary.Operations[index]
		if operation.Kind != kind || operation.Resource.Indirect || operation.Resource.Value != function.Params[parameter] || operation.Site != call.Pos() {
			t.Errorf("%s: operation %d lost order, exact argument or call-site provenance: %+v", function, index, operation)
		}
	}
}

func assertConcurrencyCaller(t *testing.T, provider *Provider, engine *concurrencyfacts.Engine, function *ssa.Function) {
	t.Helper()
	// Run executes in the analysis driver's goroutine. Assertions must return
	// normally so a failing control still completes the driver's action.
	call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
	bound, available := provider.ConcurrencyAtCall(call, proofs.NewSearchBudget(2000))
	if available != Available {
		t.Errorf("%s: selected component unavailable", function)
		return
	}
	domain := engine.AtCall(call, proofs.NewSearchBudget(2000))
	if bound.Reason != domain.Reason || bound.Complete() != domain.Complete() || len(bound.Operations) != len(domain.Operations) {
		t.Errorf("%s: broker/domain outcomes differ: %+v / %+v", function, bound, domain)
		return
	}
	if function.Name() == "Opaque" {
		if bound.Complete() || len(bound.Operations) != 0 {
			t.Error("missing imported declaration must remain incomplete")
		}
	} else {
		assertPairBinding(t, function, call, bound)
	}
	for index, operation := range bound.Operations {
		if operation.Resource != domain.Operations[index].Resource || operation.Kind != domain.Operations[index].Kind {
			t.Errorf("%s: operation %d differs from domain binding", function, index)
			return
		}
	}
	if function.Name() != "Opaque" {
		if len(bound.Operations) == 0 {
			return
		}
		bound.Operations[0].Resource.Value = nil
		bound.Operations[0].Kind = concurrencyfacts.Unlock
		fresh, _ := provider.ConcurrencyAtCall(call, proofs.NewSearchBudget(2000))
		assertPairBinding(t, function, call, fresh)
	}
	budget := proofs.NewSearchBudget(1)
	cut, _ := provider.ConcurrencyAtCall(call, budget)
	if function.Name() != "Opaque" && (cut.Complete() || !budget.Exhausted()) {
		t.Error("interrupted binding must not expose a complete effect prefix")
	}
	for allowance := 2; allowance <= 16; allowance++ {
		budget := proofs.NewSearchBudget(allowance)
		cut, _ := provider.ConcurrencyAtCall(call, budget)
		if budget.Exhausted() && (cut.Complete() || len(cut.Operations) != 0) {
			t.Errorf("%s: allowance %d exposed interrupted effects", function, allowance)
		}
	}
}
