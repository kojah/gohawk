package resourcelifetime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedWrapperAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "wrapperbudget", `package wrapperbudget
type resource struct{}
type owner struct{}
func wrap(any) *owner
func direct(p, other *resource) *owner { return wrap(p) }
func nested(p, other *resource) *owner { return wrap(wrap(p)) }
func unrelated(p, other *resource) *owner { return wrap(other) }
func tooDeep(p, other *resource) *owner { return wrap(wrap(wrap(wrap(wrap(p))))) }
`)
	fact := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Holds:   []heapmodel.HeapHold{{Result: 0, Parameter: 0, Must: true}},
	}}
	// Synthetic publication needs its declaration signature for heap claim gates.
	fact.DescribeFact(pkg.Func("wrap").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("wrap"): fact},
	}}
	provider := resourceSummaries.Provider(pass)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	for _, test := range []struct {
		name string
		want int
	}{{"direct", 0}, {"nested", 0}, {"unrelated", -1}, {"tooDeep", -1}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := &resourceAnalysis{function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence}
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			if got := query.returnedWrapperPosition(returned); got != test.want {
				t.Fatalf("default wrapper result = %d, want %d", got, test.want)
			}
			for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
				pool := ssaflow.NewSearchBudget(limit)
				budget := pool.Within(ssaflow.SummaryBudget)
				got := query.returnedWrapperPositionWithin(returned, budget)
				if budget.Exhausted() {
					if got >= 0 {
						t.Fatalf("allowance %d retained interrupted wrapper: %d", limit, got)
					}
					continue
				}
				if got != test.want {
					t.Fatalf("complete wrapper result = %d at allowance %d", got, limit)
				}
				return
			}
			t.Fatal("wrapper never completed")
		})
	}
}

func TestReturnedViewProjectionAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "projectionbudget", `package projectionbudget
type resource struct{}
type view struct{}
func (*view) Close() {}
func makeView(*resource) *view
func caller(p *resource) *view { return makeView(p) }
`)
	fn := pkg.Func("caller")
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	for _, declared := range []bool{false, true} {
		facts := lifecyclefacts.Summaries{}
		if declared {
			facts[pkg.Func("makeView")] = lifecyclefacts.Fact{Must: lifecyclefacts.MustClaims{ReturnedView: 1}}
		}
		pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{lifecyclefacts.Analyzer: facts}}
		provider := resourceSummaries.Provider(pass)
		query := &resourceAnalysis{
			function: fn, resource: fn.Params[0], summaries: provider,
			contract: resourceContract{cleanup: []string{"Close"}},
		}
		completed := false
		for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
			pool := ssaflow.NewSearchBudget(limit)
			budget := pool.Within(ssaflow.SummaryBudget)
			got := query.proveReturnedProjection(returned, returned.Results[0], budget)
			if budget.Exhausted() {
				if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
					t.Fatalf("declared %v, allowance %d retained cutoff: %+v", declared, limit, got)
				}
				continue
			}
			if got.State == ssaflow.EvidenceUnknown || got.Proven() == declared {
				t.Fatalf("declared %v, completed projection = %+v", declared, got)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("projection never completed")
		}
	}
}

func TestReturnedViewProjectionPreservesBindingCutoff(t *testing.T) {
	var source strings.Builder
	source.WriteString(`package projectioncap
type resource struct{}
type a *resource; type b *resource
type view struct{}
func (*view) Close() {}
func makeView(a) *view
func caller(p a) *view {
`)
	previous := "p"
	for index := range ssaflow.QueryBudget + 10 {
		target := "b"
		if index%2 != 0 {
			target = "a"
		}
		name := fmt.Sprintf("x%d", index)
		fmt.Fprintf(&source, "%s := %s(%s)\n", name, target, previous)
		previous = name
	}
	fmt.Fprintf(&source, "return makeView(%s)\n}\n", previous)
	pkg := ssaflowtest.BuildPackage(t, "projectioncap", source.String())
	fn := pkg.Func("caller")
	if count := len(ssaflow.InstructionsOf[*ssa.ChangeType](fn)); count <= ssaflow.QueryBudget {
		t.Fatalf("fixture has only %d SSA conversions", count)
	}
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{
			pkg.Func("makeView"): {Must: lifecyclefacts.MustClaims{ReturnedView: 1}},
		},
	}}
	query := &resourceAnalysis{
		function: fn, resource: fn.Params[0], summaries: resourceSummaries.Provider(pass),
		contract: resourceContract{cleanup: []string{"Close"}},
	}
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	budget := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	got := query.proveReturnedProjection(returned, returned.Results[0], budget)
	if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || budget.Exhausted() {
		t.Fatalf("binding cutoff became method-set acceptance: %+v, parent exhausted %v", got, budget.Exhausted())
	}
}
