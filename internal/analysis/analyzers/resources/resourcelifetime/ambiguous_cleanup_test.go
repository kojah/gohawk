package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAmbiguousCleanupAllowance(t *testing.T) {
	pkg := ambiguousCleanupFixture(t)
	for _, test := range []struct {
		name   string
		want   bool
		reason resourceLifetimeReason
	}{
		{"direct", true, resourceReasonAmbiguousCleanupValue},
		{"merged", true, resourceReasonAmbiguousCleanupValue},
		{"helper", true, resourceReasonAmbiguousHelperCleanupValue},
		{"projection", true, resourceReasonAmbiguousHelperCleanupValue},
		{"other", false, resourceReasonUntouched},
		{"readOnly", false, resourceReasonUntouched},
		{"conditional", false, resourceReasonUntouched},
		{"exactHelper", false, resourceReasonUntouched},
		{"overwritten", false, resourceReasonUntouched},
		{"wrongOrigin", false, resourceReasonUntouched},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, _ := ambiguousCleanupInputs(t, pkg.Func(test.name))
			prove := func(budget *proofs.SearchBudget) resourceProof {
				fresh, selected := ambiguousCleanupInputs(t, pkg.Func(test.name))
				return fresh.proveAmbiguousCleanupWithin(selected, selected.Common(), budget)
			}
			baseline := prove(nil)
			if baseline.Proven() != test.want || baseline.Reason != test.reason {
				t.Fatalf("default ambiguous proof = %+v; SSA:\n%s", baseline, carriedSSA(t, query.function))
			}
			checkResourceProofAllowance(t, prove, test.want)
		})
	}
}

func TestAmbiguousCleanupChildAndFresh(t *testing.T) {
	query, call := ambiguousCleanupInputs(t, ambiguousCleanupFixture(t).Func("helper"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	cut := query.proveAmbiguousCleanupWithin(call, call.Common(), pool.Within(1))
	if cut.State != proofs.EvidenceUnknown || cut.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("child proof = %+v, parent exhausted %v", cut, pool.Exhausted())
	}
	fresh := query.proveAmbiguousCleanupWithin(call, call.Common(), pool.Within(releaseSearchBudget))
	if !fresh.Proven() || fresh.Reason != resourceReasonAmbiguousHelperCleanupValue {
		t.Fatalf("fresh proof = %+v", fresh)
	}
}

func TestAmbiguousCleanupMetadataExclusions(t *testing.T) {
	query, call := ambiguousCleanupInputs(t, ambiguousCleanupFixture(t).Func("helper"))
	query.optional.proof = resourceProof{State: proofs.EvidenceProven}
	if got := query.proveAmbiguousCleanupWithin(call, call.Common(), proofs.NewSearchBudget(0)); got.State != proofs.EvidenceDisproven {
		t.Fatalf("optional acquisition queried ambiguity: %+v", got)
	}
	query.optional = optionalAcquisitionProof{}
	if got := query.proveAmbiguousCleanupWithin(call, nil, proofs.NewSearchBudget(0)); got.State != proofs.EvidenceDisproven {
		t.Fatalf("noncall queried ambiguity: %+v", got)
	}
}

func TestAmbiguousCleanupFlow(t *testing.T) {
	pkg := ambiguousCleanupFixture(t)
	for _, test := range []struct {
		name string
		want proofs.EvidenceState
	}{
		{"merged", proofs.EvidenceUnknown},
		{"helper", proofs.EvidenceUnknown},
		{"direct", proofs.EvidenceDisproven},
		{"conditional", proofs.EvidenceProven},
		{"readOnly", proofs.EvidenceProven},
		{"overwritten", proofs.EvidenceProven},
		{"other", proofs.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, _ := ambiguousCleanupInputs(t, pkg.Func(test.name))
			got := evaluateResourceFlow(nil, query.evidence, query.acquisition, query.resource, query.contract)
			if got.state != test.want || (got.leak != nil) != (test.want == proofs.EvidenceProven) {
				t.Fatalf("flow = %+v, want state %v", got, test.want)
			}
		})
	}
}

func ambiguousCleanupInputs(t *testing.T, fn *ssa.Function) (*resourceAnalysis, *ssa.Call) {
	t.Helper()
	var acquisition, selected *ssa.Call
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		switch ssaflow.CallName(call.Common()) {
		case "acquire", "acquireOwner":
			acquisition = call
		default:
			selected = call
		}
	}
	if acquisition == nil || selected == nil {
		t.Fatal("missing ambiguous cleanup inputs")
	}
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	return &resourceAnalysis{
		function: fn, resource: acquisition, acquisition: acquisition, evidence: evidence, summaries: provider,
		contract: resourceContract{cleanup: []string{"Close"}},
	}, selected
}

func ambiguousCleanupFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "ambiguouscleanup", `package ambiguouscleanup
 type resource struct{n int}
 func (*resource) Close(){}
 type owner struct{value *resource}
 func acquire()*resource{return &resource{}}
 func acquireOwner()*owner{return &owner{value:&resource{}}}
 func finish(p *resource){p.Close()}
 func inspect(p *resource){_=p.n}
 func maybe(p *resource,flag bool){if flag{p.Close()}}
 func direct(){p:=acquire();p.Close()}
 func merged(other *resource,flag bool){p:=acquire();q:=p;if flag{q=other};q.Close()}
 func helper(other *resource,flag bool){p:=acquire();q:=p;if flag{q=other};finish(q)}
 func projection(other *owner,flag bool){p:=acquireOwner();q:=p;if flag{q=other};finish(q.value)}
 func other(other *resource){p:=acquire();other.Close();_=p.n}
 func readOnly(other *resource,flag bool){p:=acquire();q:=p;if flag{q=other};inspect(q)}
 func conditional(other *resource,flag,closeIt bool){p:=acquire();q:=p;if flag{q=other};maybe(q,closeIt)}
 func exactHelper(){p:=acquire();finish(p)}
 func overwritten(other *resource){p:=acquire();o:=owner{value:p};o.value=other;finish(o.value)}
 func wrongOrigin(a,b *resource,flag bool){p:=acquire();q:=a;if flag{q=b};finish(q);_=p.n}
 `)
}
