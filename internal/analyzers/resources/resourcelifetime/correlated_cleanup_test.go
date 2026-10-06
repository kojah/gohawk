package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCorrelatedCleanupAllowance(t *testing.T) {
	pkg := correlatedCleanupFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"paired", true},
		{"tested", true},
		{"reversed", true},
		{"thirdResult", false},
		{"uncompared", false},
		{"testedBefore", false},
		{"readOnly", false},
		{"flagOnly", false},
		{"wrongResource", false},
		{"wrapped", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, _ := correlatedCleanupInputs(t, pkg.Func(test.name))
			prove := func(budget *proofs.SearchBudget) resourceProof {
				fresh, selected := correlatedCleanupInputs(t, pkg.Func(test.name))
				return fresh.provePairedErrorCleanupWithin(selected, selected.Common(), budget)
			}
			baseline := prove(nil)
			if baseline.Proven() != test.want || baseline.State == proofs.EvidenceUnknown {
				t.Fatalf("proof=%+v; SSA:\n%s", baseline, carriedSSA(t, query.function))
			}
			if test.want && baseline.Reason != resourceReasonPairedErrorHelperCleanup {
				t.Fatalf("reason=%v", baseline.Reason)
			}
			checkResourceProofAllowance(t, prove, test.want)
		})
	}
}

func TestCorrelatedCleanupChildAndFresh(t *testing.T) {
	query, call := correlatedCleanupInputs(t, correlatedCleanupFixture(t).Func("tested"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	cut := query.provePairedErrorCleanupWithin(call, call.Common(), pool.Within(1))
	if cut.State != proofs.EvidenceUnknown || cut.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("cut=%+v, parent exhausted=%v", cut, pool.Exhausted())
	}
	fresh := query.provePairedErrorCleanupWithin(call, call.Common(), pool.Within(releaseSearchBudget))
	if !fresh.Proven() {
		t.Fatalf("fresh=%+v", fresh)
	}
	if got := query.provePairedErrorCleanupWithin(call, nil, proofs.NewSearchBudget(0)); got.State != proofs.EvidenceDisproven {
		t.Fatalf("noncall=%+v", got)
	}
}

func TestCorrelatedCleanupFlow(t *testing.T) {
	pkg := correlatedCleanupFixture(t)
	for _, test := range []struct {
		name string
		want proofs.EvidenceState
	}{
		{"paired", proofs.EvidenceUnknown},
		{"tested", proofs.EvidenceUnknown},
		{"readOnly", proofs.EvidenceProven},
		{"flagOnly", proofs.EvidenceProven},
		{"uncompared", proofs.EvidenceProven},
		{"testedBefore", proofs.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, _ := correlatedCleanupInputs(t, pkg.Func(test.name))
			got := evaluateResourceFlow(nil, query.evidence, query.acquisition, query.resource, query.contract)
			if got.state != test.want || (got.leak != nil) != (test.want == proofs.EvidenceProven) {
				t.Fatalf("flow=%+v, want=%v; SSA:\n%s", got, test.want, carriedSSA(t, query.function))
			}
		})
	}
}

func correlatedCleanupInputs(t *testing.T, fn *ssa.Function) (*resourceAnalysis, *ssa.Call) {
	t.Helper()
	var acquisition, selected *ssa.Call
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		switch ssaflow.CallName(call.Common()) {
		case "acquire", "acquireThree":
			acquisition = call
		case "finish", "inspect", "flagFinish":
			selected = call
		}
	}
	if acquisition == nil || selected == nil {
		t.Fatal("missing cleanup inputs")
	}
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	return &resourceAnalysis{
		function: fn, acquisition: acquisition, resource: ssaflow.CallResult(acquisition, 0),
		evidence: evidence, summaries: provider, contract: resourceContract{cleanup: []string{"Close"}},
	}, selected
}

func correlatedCleanupFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "correlatedcleanup", `package correlatedcleanup
 type resource struct{n int}
 func (*resource)Close(){}
 func acquire()(*resource,error){return &resource{},nil}
 func acquireThree()(*resource,int,error){return &resource{},0,nil}
 func finish(p *resource,e error){if e!=nil{p.Close()}}
 func inspect(p *resource,e error){_=p.n;_=e}
 func flagFinish(p *resource,flag bool){if flag{p.Close()}}
 func wrap(e error)error{return e}
 func paired(){p,e:=acquire();finish(p,e)}
 func tested(other error){p,e:=acquire();if e!=nil{return};finish(p,other);if other!=nil{return}}
 func reversed(other error){p,e:=acquire();if e!=nil{return};finish(p,other);if nil==other{return}}
 func thirdResult(){p,_,e:=acquireThree();finish(p,e)}
 func uncompared(other error){p,e:=acquire();if e!=nil{return};finish(p,other)}
 func testedBefore(other error){p,e:=acquire();if e!=nil{return};if other!=nil{return};finish(p,other)}
 func readOnly(){p,e:=acquire();inspect(p,e)}
 func flagOnly(flag bool){p,e:=acquire();if e!=nil{return};flagFinish(p,flag)}
 func wrongResource(other *resource){p,e:=acquire();finish(other,e);_=p.n}
 func wrapped(){p,e:=acquire();finish(p,wrap(e))}
 `)
}
