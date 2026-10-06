package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestOptionalAcquisitionAllowance(t *testing.T) {
	pkg := optionalAcquisitionFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"exact", true},
		{"inverse", true},
		{"leak", true},
		{"otherGuard", false},
		{"otherResource", false},
		{"otherError", false},
		{"boxedError", false},
		{"cyclic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call, resource, errValue := acquiredResourceInputs(t, pkg.Func(test.name))
			baseline := proveOptionalAcquisitionWithin(call, resource, errValue, nil)
			if baseline.Proven() != test.want || baseline.proof.State == proofs.EvidenceUnknown {
				t.Fatalf("default diamond proof = %+v; SSA:\n%s", baseline, carriedSSA(t, call.Parent()))
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveOptionalAcquisitionWithin(call, resource, errValue, budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted ||
						got.resourcePhi != nil || got.merge != nil || got.acquisitionBlock != nil || got.acquiredSuccessor != nil {
						t.Fatalf("interrupted diamond retains correlation: %+v", got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete diamond = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("diamond proof never completed")
		})
	}
}

func TestOptionalAcquisitionChildCutoff(t *testing.T) {
	call, resource, errValue := acquiredResourceInputs(t, optionalAcquisitionFixture(t).Func("exact"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := proveOptionalAcquisitionWithin(call, resource, errValue, pool.Within(2))
	if got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted || got.resourcePhi != nil || pool.Exhausted() {
		t.Fatalf("optional child cutoff = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	if fresh := proveOptionalAcquisitionWithin(call, resource, errValue, pool.Within(releaseSearchBudget)); !fresh.Proven() {
		t.Fatalf("fresh diamond = %+v", fresh)
	}
}

func TestOptionalAcquisitionFlow(t *testing.T) {
	pkg := optionalAcquisitionFixture(t)
	for _, name := range []string{"exact", "inverse", "leak"} {
		t.Run(name, func(t *testing.T) {
			call, resource, _ := acquiredResourceInputs(t, pkg.Func(name))
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			got := evaluateResourceFlow(nil, evidence, call, resource, resourceContract{cleanup: []string{"Close"}})
			if name == "leak" {
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("optional leak lost = %+v", got)
				}
			} else if got.state != proofs.EvidenceDisproven || got.leak != nil {
				t.Fatalf("optional cleanup reported = %+v", got)
			}
		})
	}
}

func acquiredResourceInputs(t *testing.T, fn *ssa.Function) (*ssa.Call, ssa.Value, ssa.Value) {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) != "acquire" {
			continue
		}
		var resource, errValue ssa.Value
		for _, extract := range ssaflow.InstructionsOf[*ssa.Extract](fn) {
			if extract.Tuple != call {
				continue
			}
			switch extract.Index {
			case 0:
				resource = extract
			case 1:
				errValue = extract
			}
		}
		if resource == nil || errValue == nil {
			t.Fatal("missing acquisition results")
		}
		return call, resource, errValue
	}
	t.Fatal("missing acquisition")
	return nil, nil, nil
}

func optionalAcquisitionFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "optionalacquisition", `package optionalacquisition
 type resource struct{n int}
 func (*resource) Close(){}
 func acquire()(*resource,error){return &resource{},nil}
 func exact(flag int){var p *resource;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 func inverse(flag int){var p *resource;var e error;if flag!=0{p,e=acquire()};if flag==0{return};if e!=nil{return};p.Close()}
 func leak(flag int){var p *resource;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.n++}}
 func otherGuard(flag,other int){var p *resource;var e error;if flag==1{p,e=acquire()};if other==1{if e!=nil{return};p.Close()}}
 func otherResource(flag int,other *resource){p:=other;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 func otherError(flag int,other error){var p *resource;e:=other;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 type failure struct{}
 func (*failure) Error()string{return "failure"}
 func boxedError(flag int){var p *resource;var e error=(*failure)(nil);if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 func cyclic(flag int,done bool){for{var p *resource;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()};if done{return}}}
 `)
}
