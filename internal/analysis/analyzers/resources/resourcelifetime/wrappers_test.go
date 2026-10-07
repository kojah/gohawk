package resourcelifetime

import (
	"fmt"
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

func TestPossibleWrapperAllowance(t *testing.T) {
	pkg := possibleWrapperFixture(t)
	for _, test := range []struct {
		name   string
		depth  int
		direct bool
		want   bool
	}{
		{"direct", 0, false, false},
		{"direct", 0, true, true},
		{"nested", 0, false, true},
		{"chain", 0, false, false},
		{"chain", 4, false, true},
		{"unrelated", 4, true, false},
		{"scalar", 4, true, false},
		{"failure", 4, true, false},
		{"borrowed", 4, true, false},
		{"deep", 4, false, false},
		{"explicit", 4, false, true},
		{"spread", 4, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
			query := aggregateEscapeAnalysis(fn)
			if got := query.provePossibleWrapperWithin(value, test.depth, test.direct, nil); got.Proven() != test.want {
				t.Fatalf("default wrapper = %+v, want %v; SSA:\n%s", got, test.want, carriedSSA(t, fn))
			}
			checkPossibleWrapperAllowance(t, query, value, test.depth, test.direct, test.want)
		})
	}
}

func checkPossibleWrapperAllowance(t *testing.T, query *resourceAnalysis, value ssa.Value, depth int, direct, want bool) {
	t.Helper()
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		proof := query.provePossibleWrapperWithin(value, depth, direct, budget)
		if resourceFlowExhausted(budget) || limit == 0 {
			if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
				t.Fatalf("allowance %d retained interrupted wrapper: %+v", limit, proof)
			}
			continue
		}
		if proof.State == proofs.EvidenceUnknown || proof.Proven() != want {
			t.Fatalf("complete wrapper = %+v, want %v", proof, want)
		}
		return
	}
	t.Fatal("wrapper never completed")
}

func TestWrapperPublicationAllowance(t *testing.T) {
	pkg := possibleWrapperFixture(t)
	fact := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Effects: []heapmodel.HeapEffect{{
			Slot:   heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: 0}},
			Escape: heapmodel.HeapEscapedGlobal, Every: true,
		}},
	}}
	fact.DescribeFact(pkg.Func("keep").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("keep"): fact},
	}}
	provider := resourceSummaries.Provider(pass)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	for _, test := range []struct {
		name string
		want bool
	}{{"kept", true}, {"used", false}, {"foreign", true}, {"local", false}, {"directstore", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := &resourceAnalysis{function: fn, resource: fn.Params[0], evidence: evidence, summaries: provider}
			checkWrapperPublicationAllowance(t, query, test.want)
		})
	}
}

func checkWrapperPublicationAllowance(t *testing.T, query *resourceAnalysis, want bool) {
	t.Helper()
	var last ssa.Instruction
	for instruction := range ssaflow.InstructionsWithin(query.function, nil) {
		switch instruction.(type) {
		case *ssa.Call, *ssa.Store:
			last = instruction
		}
	}
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		var proof resourceProof
		switch instruction := last.(type) {
		case *ssa.Call:
			proof = query.proveChainKeptWithin(instruction, instruction.Common(), budget)
		case *ssa.Store:
			proof = query.proveWrapperStoredOnForeignOwnerWithin(instruction, budget)
		default:
			t.Fatalf("publication instruction missing: %v", last)
		}
		if resourceFlowExhausted(budget) || limit == 0 {
			if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
				t.Fatalf("allowance %d retained interrupted publication: %+v", limit, proof)
			}
			continue
		}
		if proof.State == proofs.EvidenceUnknown || proof.Proven() != want {
			t.Fatalf("complete publication = %+v, want %v; SSA:\n%s", proof, want, carriedSSA(t, query.function))
		}
		return
	}
	t.Fatal("publication never completed")
}

func TestWrapperRetentionClassifierCutoff(t *testing.T) {
	pkg := possibleWrapperFixture(t)
	fn := pkg.Func("used")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[2]
	query := aggregateEscapeAnalysis(fn)
	query.pool = proofs.NewSearchBudget(0)
	if reason, opaque := query.opaqueFunctionCall(call, call.Common(), false); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted retaining-call classifier = %v/%v", reason, opaque)
	}
	query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
	if reason, opaque := query.opaqueFunctionCall(call, call.Common(), false); opaque || reason != resourceReasonNone {
		t.Fatalf("fresh non-retaining-call classifier = %v/%v", reason, opaque)
	}
}

func possibleWrapperFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "possiblewrapper", `package possiblewrapper
type resource struct { n int }
type owner struct { n int }
type holder struct { value any }
func wrap(any) *owner
func count(any) int
func fail(any) error
func borrow(any) *owner { return &owner{} }
func keep(any)
func use(any)
func direct(p, other *resource) any { return wrap(p) }
func nested(p, other *resource) any { return wrap(&holder{p}) }
func chain(p, other *resource) any { return wrap(wrap(p)) }
func unrelated(p, other *resource) any { return wrap(wrap(other)) }
func scalar(p, other *resource) any { return count(p) }
func failure(p, other *resource) any { return fail(p) }
func borrowed(p, other *resource) any { return borrow(p) }
func deep(p, other *resource) any { return wrap(wrap(wrap(wrap(wrap(wrap(wrap(p))))))) }
func explicit(p, other *resource) any { return append([]any(nil), wrap(p)) }
func spread(p, other *resource, items []any) any { return append([]any(nil), items...) }
func kept(p *resource) { keep(wrap(wrap(p))) }
func used(p *resource) { use(wrap(wrap(p))) }
func foreign(p *resource, dest *holder) { dest.value = wrap(p) }
func local(p *resource) *holder { h := &holder{}; h.value = wrap(p); return h }
func directstore(p *resource, dest *holder) { dest.value = p }
`)
}

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
			if got := query.returnedWrapperPositionWithin(returned, nil); got != test.want {
				t.Fatalf("default wrapper result = %d, want %d", got, test.want)
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
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
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			pool := proofs.NewSearchBudget(limit)
			budget := pool.Within(proofs.SummaryBudget)
			got := query.proveReturnedProjection(returned, returned.Results[0], budget)
			if budget.Exhausted() {
				if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
					t.Fatalf("declared %v, allowance %d retained cutoff: %+v", declared, limit, got)
				}
				continue
			}
			if got.State == proofs.EvidenceUnknown || got.Proven() == declared {
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
	for index := range proofs.QueryBudget + 10 {
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
	if count := len(ssaflow.InstructionsOf[*ssa.ChangeType](fn)); count <= proofs.QueryBudget {
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
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	got := query.proveReturnedProjection(returned, returned.Results[0], budget)
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || budget.Exhausted() {
		t.Fatalf("binding cutoff became method-set acceptance: %+v, parent exhausted %v", got, budget.Exhausted())
	}
}

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
