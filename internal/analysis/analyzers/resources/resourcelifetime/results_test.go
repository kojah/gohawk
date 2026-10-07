package resourcelifetime

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCallResultPublicationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "publication", `package publication
type owner struct { n int }
type holder struct { value *owner }
var kept any
var count int
var failure error
func makeOwner() *owner
func makeCount() int
func makeError() error
func returned() *owner { return makeOwner() }
func boxed() any { return makeOwner() }
func field() *int { return &makeOwner().n }
func global() { kept = makeOwner() }
func scalarGlobal() { count = makeCount() }
func scalarReturn() int { return makeCount() }
func errorGlobal() { failure = makeError() }
func errorReturn() error { return makeError() }
func unrelated() *owner { _ = makeOwner(); return &owner{} }
func discarded() { _ = makeOwner() }
func foreign(dest *holder) { dest.value = makeOwner() }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"returned", true},
		{"boxed", true},
		{"field", true},
		{"global", true},
		{"scalarGlobal", false},
		{"scalarReturn", true},
		{"errorGlobal", false},
		{"errorReturn", false},
		{"unrelated", false},
		{"discarded", false},
		{"foreign", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return proveCallResultMayTransferWithin(call, budget)
			}, test.want)
		})
	}
}

func TestResultPublicationCensusCutoff(t *testing.T) {
	pkg := resultPublicationCensusFixture(t)
	fn := pkg.Func("long")
	calls := ssaflow.InstructionsOf[*ssa.Call](fn)
	if len(calls) != proofs.SummaryBudget+101 {
		t.Fatalf("call census = %d, want %d", len(calls), proofs.SummaryBudget+101)
	}
	budget := proofs.NewSearchBudget(proofs.SummaryBudget)
	if proof := proveCallResultMayTransferWithin(calls[0], budget); proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted publication census = %+v", proof)
	}
	if fresh := proveCallResultMayTransferWithin(calls[0], proofs.NewSearchBudget(10*proofs.SummaryBudget)); !fresh.Proven() {
		t.Fatalf("fresh publication census = %+v", fresh)
	}
}

func TestResultPublicationClassifierCutoff(t *testing.T) {
	pkg := resultPublicationCensusFixture(t)
	fn := pkg.Func("long")
	calls := ssaflow.InstructionsOf[*ssa.Call](fn)
	query := aggregateEscapeAnalysis(fn)
	query.pool = proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	if reason, opaque := query.opaqueFunctionCall(calls[0], calls[0].Common(), true); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("publication child-cutoff classifier = %v/%v", reason, opaque)
	}
	if resourceFlowExhausted(query.pool) {
		t.Fatal("publication child cutoff consumed the whole candidate pool")
	}
	short := pkg.Func("short")
	call := ssaflow.InstructionsOf[*ssa.Call](short)[0]
	shortQuery := aggregateEscapeAnalysis(short)
	if reason, opaque := shortQuery.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonNestedInTransferredArgument {
		t.Fatalf("complete publication classifier = %v/%v", reason, opaque)
	}
}

func resultPublicationCensusFixture(t *testing.T) *ssa.Package {
	t.Helper()
	var source strings.Builder
	source.WriteString(`package publicationcensus
type resource struct { n int }
type holder struct { value *resource }
type owner struct { n int }
func view(*holder) *owner { return &owner{} }
func noise() {}
func long(p *resource) *owner { result := view(&holder{p})
`)
	for range proofs.SummaryBudget + 100 {
		source.WriteString("noise()\n")
	}
	source.WriteString("return result\n}\nfunc short(p *resource) *owner { return view(&holder{p}) }")
	return ssaflowtest.BuildPackage(t, "publicationcensus", source.String())
}

func TestResourceResultGuardDiscoveryAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "resourceguards", `package resourceguards
 type resource struct{}
 func (*resource) Close(){}
 func acquire()(*resource,error){return &resource{},nil}
 func guarded()(err error){p,err:=acquire();if err!=nil{return err};defer func(){if err!=nil{p.Close()}}();return nil}
 func success()(err error){p,err:=acquire();if err!=nil{return err};defer func(){if err==nil{p.Close()}}();return nil}
 `)
	for _, name := range []string{"guarded", "success"} {
		t.Run(name, func(t *testing.T) {
			call, resource, _ := acquiredResourceInputs(t, pkg.Func(name))
			for limit := range proofs.SummaryBudget {
				query := &resourceAnalysis{function: call.Parent(), resource: resource, contract: resourceContract{cleanup: []string{"Close", "Close"}}}
				budget := proofs.NewSearchBudget(limit)
				got := query.discoverResultGuardedDefersWithin(budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || query.guardedDefers != nil {
						t.Fatalf("cut %d = %+v, guards %+v", limit, got, query.guardedDefers)
					}
					continue
				}
				if !got.Proven() || len(query.guardedDefers) != 1 {
					t.Fatalf("complete discovery = %+v, guards %+v", got, query.guardedDefers)
				}
				fresh := query.discoverResultGuardedDefersWithin(proofs.NewSearchBudget(proofs.SummaryBudget))
				if !fresh.Proven() || len(query.guardedDefers) != 1 {
					t.Fatalf("fresh repeated discovery = %+v, guards %+v", fresh, query.guardedDefers)
				}

				provider := resourceSummaries.Provider(nil)
				evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
				flow := evaluateResourceFlow(nil, evidence, call, resource, query.contract)
				if name == "guarded" {
					if flow.state != proofs.EvidenceProven || flow.leak == nil {
						t.Fatalf("guarded leak lost: %+v", flow)
					}
				} else if flow.state != proofs.EvidenceDisproven || flow.leak != nil {
					t.Fatalf("success cleanup reported: %+v", flow)
				}
				return
			}
			t.Fatal("resource discovery never completed")
		})
	}
}

func TestResourceResultGuardReturnCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnguards", `package returnguards
 type resource struct{}
 func(*resource)Close(){}
 func acquire()(*resource,error){return &resource{},nil}
 func skipped()(err error){p,_:=acquire();defer func(){if err!=nil{p.Close()}}();return nil}
 func released()(err error){p,_:=acquire();defer func(){if err==nil{p.Close()}}();return nil}
 `)
	for _, name := range []string{"skipped", "released"} {
		fn := pkg.Func(name)
		call, resource, _ := acquiredResourceInputs(t, fn)
		query := &resourceAnalysis{function: call.Parent(), resource: resource, contract: resourceContract{cleanup: []string{"Close"}}}
		if proof := query.discoverResultGuardedDefersWithin(nil); !proof.Proven() {
			t.Fatalf("discovery=%+v", proof)
		}
		for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
			if !cfg.InstructionDominates(query.guardedDefers[0].Defer, returned) {
				continue
			}
			query.pool = proofs.NewSearchBudget(0)
			action, reason, ok := query.resultGuardedReturn(returned)
			if !ok || action != actionUnknown || reason != resourceReasonResultGuardedUnknown {
				t.Fatalf("cut=%v/%v/%v", action, reason, ok)
			}
			query.pool = proofs.NewSearchBudget(resourcePoolBudget)
			action, _, ok = query.resultGuardedReturn(returned)
			if name == "released" && (!ok || action != actionSettled) {
				t.Fatalf("fresh release=%v/%v", action, ok)
			}
			if name == "skipped" && (ok || action != actionNone) {
				t.Fatalf("fresh skip=%v/%v", action, ok)
			}
		}
	}
}
