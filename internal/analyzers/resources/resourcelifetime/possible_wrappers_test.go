package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
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
	for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
		proof := query.provePossibleWrapperWithin(value, depth, direct, budget)
		if resourceFlowExhausted(budget) || limit == 0 {
			if proof.State != ssaflow.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
				t.Fatalf("allowance %d retained interrupted wrapper: %+v", limit, proof)
			}
			continue
		}
		if proof.State == ssaflow.EvidenceUnknown || proof.Proven() != want {
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
	for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
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
			if proof.State != ssaflow.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
				t.Fatalf("allowance %d retained interrupted publication: %+v", limit, proof)
			}
			continue
		}
		if proof.State == ssaflow.EvidenceUnknown || proof.Proven() != want {
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
	query.pool = ssaflow.NewSearchBudget(0)
	if reason, opaque := query.opaqueFunctionCall(call, call.Common(), false); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted retaining-call classifier = %v/%v", reason, opaque)
	}
	query.pool = ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
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
