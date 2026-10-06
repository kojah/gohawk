package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAcquisitionErrorResultAllowance(t *testing.T) {
	pkg := acquisitionResultFixture(t)
	for _, test := range []struct {
		name    string
		slot    int
		queried bool
	}{
		{"pair", 1, true},
		{"triple", 2, true},
		{"blank", 1, true},
		{"discarded", -1, false},
		{"unused", 1, true},
		{"scalar", -1, false},
		{"noError", -1, false},
		{"nonLast", -1, false},
		{"concreteError", -1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := acquisitionResultCall(t, pkg.Func(test.name))
			baseline := proveAcquisitionErrorResultWithin(call, nil)
			var want ssa.Value
			if test.slot >= 0 {
				want = ssacall.CallResult(call, test.slot)
			}
			if baseline.proof.State == proofs.EvidenceUnknown || baseline.value != want || baseline.proof.Proven() != (want != nil) {
				t.Fatalf("result = %+v, want %v; SSA:\n%s", baseline, want, carriedSSA(t, call.Parent()))
			}
			for limit := range 21 {
				budget := proofs.NewSearchBudget(limit)
				got := proveAcquisitionErrorResultWithin(call, budget)
				if resourceFlowExhausted(budget) {
					if !test.queried || got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted || got.value != nil {
						t.Fatalf("cut %d = %+v", limit, got)
					}
					continue
				}
				if test.queried && limit == 0 {
					t.Fatal("eligible result lookup ignored allowance")
				}
				if got != baseline {
					t.Fatalf("completed result = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("result search never completed")
		})
	}
}

func TestAcquisitionErrorResultChildCutoff(t *testing.T) {
	call := acquisitionResultCall(t, acquisitionResultFixture(t).Func("triple"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := proveAcquisitionErrorResultWithin(call, pool.Within(1))
	if got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted || got.value != nil || pool.Exhausted() {
		t.Fatalf("child cutoff = %+v, parent exhausted %v", got, pool.Exhausted())
	}
	if fresh := proveAcquisitionErrorResultWithin(call, pool.Within(releaseSearchBudget)); fresh != proveAcquisitionErrorResultWithin(call, nil) {
		t.Fatalf("fresh result = %+v", fresh)
	}
}

func TestAcquisitionErrorResultParentCutoff(t *testing.T) {
	call := acquisitionResultCall(t, acquisitionResultFixture(t).Func("pair"))
	pool := proofs.NewSearchBudget(1)
	child := pool.Within(releaseSearchBudget)
	sibling := pool.Within(releaseSearchBudget)
	sibling.Spend()
	sibling.Spend()
	got := proveAcquisitionErrorResultWithin(call, child)
	if got.proof.State != proofs.EvidenceUnknown || got.value != nil {
		t.Fatalf("exhausted shared parent supplied error result: %+v", got)
	}
}

func TestAcquisitionErrorResultFlow(t *testing.T) {
	pkg := acquisitionResultFixture(t)
	for _, name := range []string{"pair", "triple", "leak"} {
		t.Run(name, func(t *testing.T) {
			call := acquisitionResultCall(t, pkg.Func(name))
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			got := evaluateResourceFlow(nil, evidence, call, ssacall.CallResult(call, 0), resourceContract{cleanup: []string{"Close"}})
			if name == "leak" {
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("leak lost: %+v", got)
				}
			} else if got.state != proofs.EvidenceDisproven || got.leak != nil {
				t.Fatalf("successful cleanup reported: %+v", got)
			}
		})
	}
}

func acquisitionResultCall(t *testing.T, fn *ssa.Function) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) != "Close" {
			return call
		}
	}
	t.Fatal("missing acquisition")
	return nil
}

func acquisitionResultFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "acquisitionresult", `package acquisitionresult
 type resource struct{n int}
 func (*resource) Close(){}
 type failure struct{}
 func (failure) Error()string{return "failure"}
 func acquire()(*resource,error){return &resource{},nil}
 func acquireThree()(*resource,*resource,error){return &resource{},&resource{},nil}
 func acquireOne()*resource{return &resource{}}
 func acquireNoError()(*resource,int){return &resource{},0}
 func acquireNonLast()(*resource,error,int){return &resource{},nil,0}
 func acquireConcrete()(*resource,failure){return &resource{},failure{}}
 func pair(){p,e:=acquire();if e!=nil{return};p.Close()}
 func triple(){p,q,e:=acquireThree();if e!=nil{return};p.Close();q.Close()}
 func discarded(){acquire()}
 func blank(){p,_:=acquire();p.Close()}
 func unused(){p,e:=acquire();_=e;p.Close()}
 func scalar(){p:=acquireOne();p.Close()}
 func noError(){p,_:=acquireNoError();p.Close()}
 func nonLast(){p,_,_:=acquireNonLast();p.Close()}
 func concreteError(){p,_:=acquireConcrete();p.Close()}
 func leak(){p,q,e:=acquireThree();if e!=nil{return};p.n++;q.Close()}
 `)
}
