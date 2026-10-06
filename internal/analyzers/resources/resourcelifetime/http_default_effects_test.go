package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestHTTPDefaultOverridePolicies(t *testing.T) {
	pkg := httpDefaultEffectsFixture(t)
	for _, test := range []struct {
		name         string
		root, strict bool
	}{
		{"ordinary", false, false},
		{"harmlessHelper", false, false},
		{"directDo", false, true},
		{"nestedDo", true, true},
		{"clientStore", true, true},
		{"transportStore", true, true},
		{"clientField", true, true},
		{"nestedStore", true, true},
		{"recursive", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			if got := !proveDefaultClientUnmodifiedWithin(fn, nil).Proven(); got != test.root {
				t.Fatalf("HEAD root override=%v, want %v; SSA:\n%s", got, test.root, carriedSSA(t, fn))
			}
			if got := newHTTPWriterEffects().overrides.Function(fn, proofs.NewSearchBudget(httpEffectsBudget)); got != test.strict {
				t.Fatalf("strict override=%v, want %v; SSA:\n%s", got, test.strict, carriedSSA(t, fn))
			}
		})
	}
}

func TestHTTPDefaultOverrideRootPolicyDoesNotChangeMemo(t *testing.T) {
	fn := httpDefaultEffectsFixture(t).Func("directDo")
	effects := newHTTPWriterEffects()
	if !effects.overrides.Function(fn, proofs.NewSearchBudget(httpEffectsBudget)) {
		t.Fatal("strict default-effect summary lost the default-client load")
	}
	if effects.scanDefaultOverrides(fn, proofs.NewSearchBudget(httpEffectsBudget), true) {
		t.Fatal("cached strict summary intercepted the root-only Do allowance")
	}
	if !effects.overrides.Function(fn, proofs.NewSearchBudget(httpEffectsBudget)) {
		t.Fatal("root-only allowance replaced the strict declaration summary")
	}
}

func TestHTTPDefaultOverrideAllowance(t *testing.T) {
	fn := httpDefaultEffectsFixture(t).Func("harmlessHelper")
	for _, root := range []bool{false, true} {
		completed := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			budget := proofs.NewSearchBudget(limit)
			got := newHTTPWriterEffects().scanDefaultOverrides(fn, budget, root)
			if resourceFlowExhausted(budget) {
				if !got {
					t.Fatalf("interrupted default-effect scan established no override at %d", limit)
				}
				continue
			}
			if got {
				t.Fatal("complete harmless helper established an override")
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("default-effect scan never completed")
		}
	}
}

func TestHTTPDefaultOverrideFreshMemo(t *testing.T) {
	fn := httpDefaultEffectsFixture(t).Func("harmlessHelper")
	effects := newHTTPWriterEffects()
	pool := proofs.NewSearchBudget(httpEffectsBudget)
	child := pool.Within(2)
	if !effects.overrides.Function(fn, child) || !child.Exhausted() || pool.Exhausted() {
		t.Fatal("child cutoff must retain possible modification and parent availability")
	}
	fresh := pool.Within(httpEffectsBudget)
	if effects.overrides.Function(fn, fresh) || fresh.Exhausted() {
		t.Fatal("fresh allowance must recover harmless effects without cached cutoff")
	}
}

func httpDefaultEffectsFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "httpdefaults", `package httpdefaults
 import "net/http"
 func mark(){}
 func ordinary(){mark()}
 func harmlessHelper(){ordinary()}
 func directDo(request *http.Request){_,_=http.DefaultClient.Do(request)}
 func nestedDo(request *http.Request){directDo(request)}
 func clientStore(){http.DefaultClient=&http.Client{}}
 func transportStore(transport http.RoundTripper){http.DefaultTransport=transport}
 func clientField(){http.DefaultClient.Timeout=0}
 func nestedStore(){clientStore()}
 func recursive(done bool){if done{return};recursive(done)}
 `)
}
