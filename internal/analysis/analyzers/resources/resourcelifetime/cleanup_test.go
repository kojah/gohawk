package resourcelifetime

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
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
		function: fn, acquisition: acquisition, resource: ssacall.CallResult(acquisition, 0),
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

func TestPriorCleanupAllowance(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{{"captured", true}, {"registered", true}, {"unrelated", false}, {"byValue", false}, {"later", false}, {"leak", false}} {
		t.Run(test.name, func(t *testing.T) {
			query, call := priorCleanupAnalysis(t, pkg.Func(test.name))
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				got := query.provePriorCleanupWithin(call, budget)
				if got.Proven() && got.Instruction == nil {
					t.Fatal("complete prior-cleanup witness has no instruction")
				}
				return got.resourceProof
			}, test.want)
		})
	}
}

func TestCapturedCellCleanupAllowance(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, name := range []string{"captured", "unrelated", "byValue"} {
		t.Run(name, func(t *testing.T) {
			query, _ := priorCleanupAnalysis(t, pkg.Func(name))
			deferred := ssaflow.InstructionsOf[*ssa.Defer](query.function)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveCapturedCellCleanupWithin(deferred, budget)
			}, name == "captured")
		})
	}
}

func TestPriorCleanupChildCutoff(t *testing.T) {
	query, call := priorCleanupAnalysis(t, priorCleanupFixture(t).Func("captured"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.provePriorCleanupWithin(call, pool.Within(2))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || got.Instruction != nil || pool.Exhausted() {
		t.Fatalf("interrupted prior registration = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	if fresh := query.provePriorCleanupWithin(call, pool.Within(releaseSearchBudget)); !fresh.Proven() || fresh.Instruction == nil {
		t.Fatalf("fresh prior registration = %+v", fresh)
	}
}

func TestCapturedCellClassifierCutoff(t *testing.T) {
	query, _ := priorCleanupAnalysis(t, priorCleanupFixture(t).Func("captured"))
	deferred := ssaflow.InstructionsOf[*ssa.Defer](query.function)[0]
	closure := deferred.Common().Value.(*ssa.MakeClosure)
	query.pool = proofs.NewSearchBudget(0)
	if reason, opaque := query.opaqueClosureCall(deferred, closure, false); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted captured cleanup classifier = %v/%v", reason, opaque)
	}
	query.pool = proofs.NewSearchBudget(resourcePoolBudget)
	if reason, opaque := query.opaqueClosureCall(deferred, closure, false); !opaque || reason != resourceReasonCapturedCellMayCleanup {
		t.Fatalf("fresh captured cleanup classifier = %v/%v", reason, opaque)
	}
}

func TestPriorCleanupFlow(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, name := range []string{"captured", "registered", "leak", "released"} {
		t.Run(name, func(t *testing.T) {
			query, call := priorCleanupAnalysis(t, pkg.Func(name))
			got := evaluateResourceFlow(nil, query.evidence, call, call, query.contract)
			switch name {
			case "captured", "registered":
				if got.state != proofs.EvidenceUnknown || got.leak != nil {
					t.Fatalf("prior cleanup lost uncertainty = %+v", got)
				}
			case "leak":
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("leak witness lost = %+v", got)
				}
			case "released":
				if got.state != proofs.EvidenceDisproven || got.leak != nil {
					t.Fatalf("exact cleanup reported = %+v", got)
				}
			}
		})
	}
}

func priorCleanupAnalysis(t *testing.T, fn *ssa.Function) (*resourceAnalysis, *ssa.Call) {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) == "acquire" {
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			return &resourceAnalysis{
				function: fn, resource: call, summaries: provider, evidence: evidence,
				contract: resourceContract{cleanup: []string{"Close"}},
			}, call
		}
	}
	t.Fatal("missing acquisition")
	return nil, nil
}

func priorCleanupFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "priorcleanup", `package priorcleanup
import "testing"
type resource struct { n int }
func (*resource) Close() {}
func acquire() *resource { return &resource{} }
func captured(old *resource, yes bool) { p := old; defer func(){ if yes { p.Close() } }(); p = acquire() }
func registered(t *testing.T, old *resource, yes bool) { p := old; t.Cleanup(func(){ if yes { p.Close() } }); p = acquire() }
func unrelated(old *resource) { defer func(){old.Close()}(); _ = acquire() }
func byValue(old *resource) { p := old; defer p.Close(); p = acquire() }
func later(t *testing.T) { p := acquire(); t.Cleanup(func(){p.Close()}) }
func leak() { _ = acquire() }
func released() { p := acquire(); p.Close() }
`)
}

func TestPriorDeferredCompletionAllowance(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"captured", true}, {"unrelated", false}, {"byValue", false}, {"registered", false}, {"later", false}, {"leak", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, call := priorCleanupAnalysis(t, pkg.Func(test.name))
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveDeferredBeforeAcquisitionWithin(call, budget)
			}, test.want)
		})
	}
}

func TestPriorDeferredCompletionPoolCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "priordeferred", `package priordeferred
 type resource struct{ n int }
 func (*resource) Close(){}
 func acquire()*resource{return &resource{}}
 func multiple(old,other *resource,yes bool){
  p:=old
  defer other.Close()
  defer func(){if yes {p.Close()}}()
  p=acquire()
 }
 `)
	query, call := priorCleanupAnalysis(t, pkg.Func("multiple"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(2))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("prior-defer child cutoff = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	fresh := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(releaseSearchBudget))
	if !fresh.Proven() || fresh.Reason != resourceReasonPriorDeferMayRelease {
		t.Fatalf("fresh prior-defer completion = %+v; SSA:\n%s", fresh, carriedSSA(t, call.Parent()))
	}
}

func TestPriorDeferredCompletionSharesNestedBudget(t *testing.T) {
	source := `package nesteddeferred
 type resource struct{ n int }
 func (*resource) Close(){}
 func acquire()*resource{return &resource{}}
 func noop(){}
 func pending(old *resource){p:=old;defer func(){
 ` + strings.Repeat("noop()\n", 50) + `p.Close()}();p=acquire()}
 `
	pkg := ssaflowtest.BuildPackage(t, "nesteddeferred", source)
	query, call := priorCleanupAnalysis(t, pkg.Func("pending"))
	// This allowance covers the entire caller census but not its deferred body.
	// A fresh independent completion allowance would incorrectly preserve a witness.
	limit := len(ssaflow.InstructionsOf[ssa.Instruction](call.Parent())) + 1
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(limit))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("nested completion escaped allowance = %+v; SSA:\n%s", got, carriedSSA(t, call.Parent()))
	}
	if fresh := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(releaseSearchBudget)); !fresh.Proven() {
		t.Fatalf("fresh deferred completion = %+v", fresh)
	}
}

// These assertions test the boundary itself: ordinary ownership classification
// can already decline replaced/escaped cells, so diagnostic absence alone would
// not establish that this narrower rule rejects a stale capture.
func TestCapturedBodyCleanupRejectsStaleOrigins(t *testing.T) {
	pkg := guardedBodyFixture(t)
	for _, name := range []string{"stable", "replaced", "opaqueCell", "opaqueOwner", "mapEscape", "booleanGuard", "otherBody", "fieldReplacement"} {
		t.Run(name, func(t *testing.T) {
			query, invocation, closure := guardedBodyInputs(t, pkg.Func(name))
			proof := query.proveGuardedCapturedBodyWithin(invocation, closure, nil)
			want := proofs.EvidenceDisproven
			if name == "stable" {
				want = proofs.EvidenceUnknown
			}
			if proof.State != want {
				t.Fatalf("captured cleanup = %+v, want state %v", proof, want)
			}
		})
	}
}

func guardedBodyFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "example.com/capturetest", `package capturetest
import "net/http"
var responses map[string]*http.Response
func acquire() *http.Response { return new(http.Response) }
func stable() {
 response := acquire()
 closeResponse := func() { if response.Body != nil { response.Body.Close() } }
 closeResponse()
}
func replaced(other *http.Response) {
 response := acquire()
 closeResponse := func() { if response.Body != nil { response.Body.Close() } }
 response = other
 closeResponse()
}
func opaqueCell(change func(**http.Response)) {
 response := acquire()
 closeResponse := func() { if response.Body != nil { response.Body.Close() } }
 change(&response)
 closeResponse()
}
func opaqueOwner(change func(*http.Response)) {
 response := acquire()
 closeResponse := func() { change(response); if response.Body != nil { response.Body.Close() } }
 closeResponse()
}
func mapEscape() {
 response := acquire()
 closeResponse := func() { if response.Body != nil { response.Body.Close() } }
 responses["current"] = response
 closeResponse()
}
func booleanGuard(flag bool) {
 response:=acquire()
 closeResponse:=func(){if flag && response.Body!=nil{response.Body.Close()}}
 closeResponse()
}
func otherBody(other *http.Response) {
 response:=acquire()
 closeResponse:=func(){if other.Body!=nil{other.Body.Close()};_=response.StatusCode}
 closeResponse()
}
func fieldReplacement(other interface{Read([]byte)(int,error);Close()error}) {
 response:=acquire()
 closeResponse:=func(){response.Body=other;if response.Body!=nil{response.Body.Close()}}
 closeResponse()
}
`)
}

func guardedBodyInputs(t *testing.T, function *ssa.Function) (*resourceAnalysis, *ssa.Call, *ssa.MakeClosure) {
	t.Helper()
	var acquisition, invocation *ssa.Call
	var closure *ssa.MakeClosure
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		if ssaflow.CallName(call.Common()) == "acquire" {
			acquisition = call
		}
		if created, ok := call.Common().Value.(*ssa.MakeClosure); ok {
			invocation, closure = call, created
		}
	}
	if acquisition == nil || invocation == nil || closure == nil {
		t.Fatal("missing captured cleanup SSA")
	}
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	return &resourceAnalysis{
		function: function, acquisition: acquisition, resource: acquisition, evidence: evidence, summaries: provider,
		contract: resourceContract{family: resourceFamilyHTTP, cleanup: []string{"Close"}},
	}, invocation, closure
}

func TestGuardedBodyAllowance(t *testing.T) {
	pkg := guardedBodyFixture(t)
	for _, name := range []string{"stable", "replaced", "opaqueCell", "opaqueOwner", "mapEscape", "booleanGuard", "otherBody", "fieldReplacement"} {
		t.Run(name, func(t *testing.T) {
			var completed bool
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				query, call, closure := guardedBodyInputs(t, pkg.Func(name))
				budget := proofs.NewSearchBudget(limit)
				got := query.proveGuardedCapturedBodyWithin(call, closure, budget)
				if budget.Exhausted() || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				want := proofs.EvidenceDisproven
				if name == "stable" {
					want = proofs.EvidenceUnknown
				}
				if got.State != want || got.Reason == resourceReasonBudgetExhausted {
					t.Fatalf("complete%d=%+v, want%v; SSA:\n%s", limit, got, want, carriedSSA(t, query.function))
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("query never completed")
			}
		})
	}
}

func TestGuardedBodyChildAndFresh(t *testing.T) {
	query, call, closure := guardedBodyInputs(t, guardedBodyFixture(t).Func("stable"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	cut := query.proveGuardedCapturedBodyWithin(call, closure, pool.Within(1))
	if cut.State != proofs.EvidenceUnknown || cut.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	fresh := query.proveGuardedCapturedBodyWithin(call, closure, pool.Within(proofs.SummaryBudget))
	if fresh.State != proofs.EvidenceUnknown || fresh.Reason != resourceReasonCapturedBodyGuardedCleanup {
		t.Fatalf("fresh=%+v", fresh)
	}
	query.contract.family = resourceFamilyUnknown
	if got := query.proveGuardedCapturedBodyWithin(call, closure, proofs.NewSearchBudget(0)); got.State != proofs.EvidenceDisproven {
		t.Fatalf("nonHTTP=%+v", got)
	}
}

func TestGuardedBodyFlow(t *testing.T) {
	pkg := guardedBodyFixture(t)
	for _, test := range []struct {
		name string
		want proofs.EvidenceState
	}{
		{"stable", proofs.EvidenceUnknown},
		{"booleanGuard", proofs.EvidenceProven},
		{"otherBody", proofs.EvidenceProven},
		{"fieldReplacement", proofs.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, _, _ := guardedBodyInputs(t, pkg.Func(test.name))
			got := evaluateResourceFlow(nil, query.evidence, query.acquisition, query.resource, query.contract)
			if got.state != test.want || (got.leak != nil) != (test.want == proofs.EvidenceProven) {
				t.Fatalf("flow=%+v want%v; SSA:\n%s", got, test.want, carriedSSA(t, query.function))
			}
		})
	}
}
