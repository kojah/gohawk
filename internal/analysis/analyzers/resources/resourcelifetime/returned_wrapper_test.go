package resourcelifetime

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysis/summaries"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedWrapperOpacityAllowance(t *testing.T) {
	pkg, provider := returnedWrapperFixture(t)
	for _, test := range []struct {
		name     string
		proven   bool
		position int
	}{
		{"direct", true, 0},
		{"nested", true, -1},
		{"unrelated", false, -1},
		{"discarded", false, -1},
		{"conditional", false, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			query := &resourceAnalysis{function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence}
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			baseline := query.proveReturnedWrapperWithin(returned, nil)
			if baseline.Proven() != test.proven || baseline.Position != test.position {
				t.Fatalf("default opacity = %+v, want %v/position %d", baseline, test.proven, test.position)
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
				got := query.proveReturnedWrapperWithin(returned, budget)
				if resourceFlowExhausted(budget) {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || got.Position != -1 {
						t.Fatalf("allowance %d retained interrupted wrapper: %+v", limit, got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete opacity = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("opacity never completed")
		})
	}
}

func TestReturnedWrapperOpacityRetryAndReuse(t *testing.T) {
	pkg, provider := returnedWrapperFixture(t)
	for _, name := range []string{"direct", "nested", "discarded"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			query := &resourceAnalysis{
				function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence,
				pool: proofs.NewSearchBudget(0),
			}
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			if reason, opaque := query.opaqueConsumption(returned); !opaque || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted classification = %v/%v", reason, opaque)
			}
			if len(query.wrappers) != 0 {
				t.Fatal("interrupted wrapper proof was memoized")
			}
			query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
			reason, opaque := query.opaqueConsumption(returned)
			if opaque != (name != "discarded") || (opaque && reason != resourceReasonReturnedWrapperRetains) {
				t.Fatalf("fresh classification = %v/%v", reason, opaque)
			}
			if len(query.wrappers) != 1 {
				t.Fatal("completed wrapper proof was not memoized")
			}
			budget := proofs.NewSearchBudget(0)
			got := query.returnedWrapperWithin(returned, budget)
			if got.State == proofs.EvidenceUnknown || budget.Exhausted() {
				t.Fatalf("completed result repeated work: %+v", got)
			}
		})
	}
}

func TestReturnedWrapperOpacityCensusCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returncensus", `package returncensus
type resource struct{}
type bundle struct{}
var sink int
func wide(p *resource, input int) *bundle {
 n := 0
`+strings.Repeat("n ^= input\n", proofs.SummaryBudget+10)+`sink = n; return nil
}
`)
	fn := pkg.Func("wide")
	if count := len(ssaflow.InstructionsOf[*ssa.BinOp](fn)); count <= proofs.SummaryBudget {
		t.Fatalf("fixture has only %d SSA operations", count)
	}
	query := &resourceAnalysis{function: fn, resource: fn.Params[0]}
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	budget := pool.Within(proofs.SummaryBudget)
	got := query.proveReturnedWrapperWithin(returned, budget)
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || !budget.Exhausted() || pool.Exhausted() {
		t.Fatalf("instruction census lost local cutoff: %+v, child/parent exhausted %v/%v", got, budget.Exhausted(), pool.Exhausted())
	}
	if fresh := query.proveReturnedWrapperWithin(returned, proofs.NewSearchBudget(10*proofs.SummaryBudget)); fresh.State != proofs.EvidenceDisproven {
		t.Fatalf("fresh census failed to recover: %+v", fresh)
	}
}

func returnedWrapperFixture(t *testing.T) (*ssa.Package, *summaries.Provider) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "returnopacity", `package returnopacity
type resource struct{}
type owner struct{}
type bundle struct { value *owner }
func wrap(any) *owner
func direct(p, other *resource, choose bool) *owner { return wrap(p) }
func nested(p, other *resource, choose bool) *bundle { return &bundle{wrap(p)} }
func unrelated(p, other *resource, choose bool) *bundle { return &bundle{wrap(other)} }
func discarded(p, other *resource, choose bool) *bundle { _ = wrap(p); return &bundle{} }
func conditional(p, other *resource, choose bool) *bundle {
 result := &bundle{}; if choose { result.value = wrap(p) }; return result
}
`)
	fact := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Holds:   []heapmodel.HeapHold{{Result: 0, Parameter: 0, Must: true}},
	}}
	fact.DescribeFact(pkg.Func("wrap").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("wrap"): fact},
	}}
	return pkg, resourceSummaries.Provider(pass)
}
