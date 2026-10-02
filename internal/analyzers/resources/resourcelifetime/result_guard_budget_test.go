package resourcelifetime

import (
	"testing"

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
			for limit := range ssaflow.SummaryBudget {
				query := &resourceAnalysis{function: call.Parent(), resource: resource, contract: resourceContract{cleanup: []string{"Close", "Close"}}}
				budget := ssaflow.NewSearchBudget(limit)
				got := query.discoverResultGuardedDefersWithin(budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || query.guardedDefers != nil {
						t.Fatalf("cut %d = %+v, guards %+v", limit, got, query.guardedDefers)
					}
					continue
				}
				if !got.Proven() || len(query.guardedDefers) != 1 {
					t.Fatalf("complete discovery = %+v, guards %+v", got, query.guardedDefers)
				}
				fresh := query.discoverResultGuardedDefersWithin(ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
				if !fresh.Proven() || len(query.guardedDefers) != 1 {
					t.Fatalf("fresh repeated discovery = %+v, guards %+v", fresh, query.guardedDefers)
				}

				provider := resourceSummaries.Provider(nil)
				evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
				flow := evaluateResourceFlow(nil, evidence, call, resource, query.contract)
				if name == "guarded" {
					if flow.state != ssaflow.EvidenceProven || flow.leak == nil {
						t.Fatalf("guarded leak lost: %+v", flow)
					}
				} else if flow.state != ssaflow.EvidenceDisproven || flow.leak != nil {
					t.Fatalf("success cleanup reported: %+v", flow)
				}
				return
			}
			t.Fatal("resource discovery never completed")
		})
	}
}
