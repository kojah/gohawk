package resourcelifetime

import (
	"testing"

	"golang.org/x/tools/go/ssa"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

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
			if !ssaflow.InstructionDominates(query.guardedDefers[0].Defer, returned) {
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
