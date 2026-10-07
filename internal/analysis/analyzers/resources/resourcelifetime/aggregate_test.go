package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestAggregateOwnerEscapeAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "aggregateescape", `package aggregateescape
type resource struct{}
type holder struct { value *resource }
var kept *holder
func retain(h *holder) { kept = h }
func inspect(h *holder) { _ = h.value }
func opaque(*holder)
func use(any)
func held(p, other *resource) { retain(&holder{p}) }
func borrowed(p, other *resource) { inspect(&holder{p}) }
func unrelated(p, other *resource) { retain(&holder{other}) }
func unreadable(p, other *resource) { opaque(&holder{p}) }
func direct(p, other *resource) { use(p) }
func callback(p, other *resource) { use(func(){ println(p) }) }
`)
	for _, test := range []struct {
		name string
		want bool
	}{{"held", true}, {"borrowed", false}, {"unrelated", false}, {"unreadable", true}, {"direct", false}, {"callback", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				query := aggregateEscapeAnalysis(fn)
				query.pool = proofs.NewSearchBudget(limit)
				budget := query.budget(proofs.SummaryBudget)
				got := query.proveAggregateOwnerEscapeWithin(call, call.Common(), budget)
				if resourceFlowExhausted(budget) {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted escape: %+v", limit, got)
					}
					continue
				}
				if got.State == proofs.EvidenceUnknown || got.Proven() != test.want {
					t.Fatalf("complete escape = %+v, want %v", got, test.want)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("escape never completed")
			}
		})
	}
}

func TestAggregateEscapeClassifierCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "escapelabel", `package escapelabel
type resource struct{}
type holder struct { value *resource }
var kept *holder
func retain(h *holder) { kept = h }
func caller(p *resource) { retain(&holder{p}) }
`)
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := aggregateEscapeAnalysis(fn)
	query.pool = proofs.NewSearchBudget(0)
	if reason, opaque := query.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted classifier = %v/%v", reason, opaque)
	}
	query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
	if reason, opaque := query.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonAggregateOwnerMayEscape {
		t.Fatalf("fresh classifier = %v/%v", reason, opaque)
	}
}

func aggregateEscapeAnalysis(fn *ssa.Function) *resourceAnalysis {
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	return &resourceAnalysis{function: fn, resource: fn.Params[0], evidence: evidence, summaries: provider}
}

func TestAggregateRetainedPathAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "aggregatepath", `package aggregatepath
type resource struct { n int }
type holder struct { first, second *resource }
func retainFirst(*holder)
func caller(p, other *resource) { retainFirst(&holder{p,other}) }
func sibling(p, other *resource) { retainFirst(&holder{other,p}) }
`)
	fact := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Effects: []heapmodel.HeapEffect{{
			Slot:   heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter}, Path: "field:0"},
			Escape: heapmodel.HeapEscapedGlobal,
		}},
	}}
	fact.DescribeFact(pkg.Func("retainFirst").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("retainFirst"): fact},
	}}
	provider := resourceSummaries.Provider(pass)
	for _, name := range []string{"caller", "sibling"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			query := callbackAnalysis(fn, provider)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveAggregateOwnerEscapeWithin(call, call.Common(), budget)
			}, name == "caller")
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveAggregateContentsEscapeWithin(call, 0, call.Common().Args[0], budget)
			}, name == "caller")
			pool := proofs.NewSearchBudget(proofs.SummaryBudget)
			proof := query.proveAggregateContentsEscapeWithin(call, 0, call.Common().Args[0], pool.Within(2))
			if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(pool) {
				t.Fatalf("contents path child cutoff = %+v, parent exhausted %v", proof, resourceFlowExhausted(pool))
			}
			query.pool = proofs.NewSearchBudget(0)
			if reason, opaque := query.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted path classifier = %v/%v", reason, opaque)
			}
			query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
			reason, opaque := query.opaqueFunctionCall(call, call.Common(), true)
			if opaque != (name == "caller") || opaque && reason != resourceReasonAggregateOwnerMayEscape {
				t.Fatalf("fresh path classifier = %v/%v", reason, opaque)
			}
		})
	}
}

// Probe the boundary independently: broader ownership rules may already decline
// a mutated response, so diagnostic absence cannot prove correct Body identity.
func TestResponseBodyAggregateHandoff(t *testing.T) {
	for _, test := range []struct {
		name, body string
		unknown    bool
	}{
		{"copiedBody", `ch <- packet{body: response.Body}`, true},
		{"nestedBody", `ch <- packet{nested: inner{body: response.Body}}`, true},
		{"savedBody", `body := response.Body; response.Body = replacement; ch <- packet{body: body}`, true},
		{"savedAggregate", `p := packet{body: response.Body}; old := p; p.body = replacement; ch <- old`, true},
		{"replacedBody", `response.Body = replacement; ch <- packet{body: response.Body}`, false},
		{"replacedAggregate", `p := packet{body: response.Body}; p.body = replacement; ch <- p`, false},
		{"unrelatedResponse", `ch <- packet{body: other.Body}`, false},
		{"responseMetadata", `_ = response.Body; ch <- packet{status: response.Status}`, false},
		{"readResponseData", `data, _ = io.ReadAll(response.Body); ch <- packet{data: data}`, false},
		{"responseData", `_ = response.Body; ch <- packet{data: data}`, false},
		{"opaqueResponse", `mutate(response); ch <- packet{body: response.Body}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "bodyhandoff", `package bodyhandoff
 import "net/http"
 import "io"
 type inner struct { body io.ReadCloser }
 type packet struct { body io.ReadCloser; nested inner; status string; data []byte }
 func acquire() *http.Response
 func mutate(*http.Response)
 func probe(ch chan packet, other *http.Response, replacement io.ReadCloser, data []byte) {
  response := acquire(); _ = response; `+test.body+`
 }
`)
			function := pkg.Func("probe")
			var resource ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
				if call.Common().StaticCallee() == pkg.Func("acquire") {
					resource = call
				}
			}
			sends := ssaflow.InstructionsOf[*ssa.Send](function)
			if resource == nil || len(sends) != 1 {
				t.Fatal("missing acquisition or handoff")
			}
			analysis := resourceAnalysis{function: function, resource: resource, contract: resourceContract{family: resourceFamilyHTTP}}
			proof := analysis.responseBodyAggregateHandoff(sends[0].X, sends[0])
			want := proofs.EvidenceDisproven
			if test.unknown {
				want = proofs.EvidenceUnknown
			}
			if proof.State != want || test.unknown && proof.Reason != resourceReasonResponseBodyAggregateHandoff {
				t.Fatalf("body handoff = %+v, want state %v", proof, want)
			}
		})
	}
}
