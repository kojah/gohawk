package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

func TestHEADAcquisitionAllowance(t *testing.T) {
	pkg := headAllowanceFixture(t, false)
	for _, test := range []struct {
		name   string
		reason resourceLifetimeReason
	}{
		{"exact", resourceReasonHeadAcquisition},
		{"cloned", resourceReasonHeadAcquisition},
		{"headers", resourceReasonHeadAcquisition},
		{"defaultClient", resourceReasonHeadAcquisition},
		{"capturedClient", resourceReasonHeadAcquisition},
		{"get", resourceReasonNone},
		{"phi", resourceReasonNone},
		{"opaque", resourceReasonNone},
		{"changed", resourceReasonHeadRequestModified},
		{"escaped", resourceReasonHeadRequestModified},
		{"timeout", resourceReasonHeadClientNotUnconfigured},
		{"modifiedCapture", resourceReasonHeadClientNotUnconfigured},
		{"defaultModified", resourceReasonHeadClientNotUnconfigured},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := headDoCall(t, pkg.Func(test.name))
			baseline := proveHeadAcquisitionWithin(call, nil)
			if baseline.Reason != test.reason {
				t.Fatalf("default HEAD=%+v, want %v; SSA:\n%s", baseline, test.reason, carriedSSA(t, call.Parent()))
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveHeadAcquisitionWithin(call, budget)
				if resourceFlowExhausted(budget) || limit == 0 && test.name != "phi" && test.name != "opaque" {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("cut HEAD=%+v", got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete HEAD=%+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("HEAD proof never completed")
		})
	}
}

func TestHEADAcquisitionChildCutoff(t *testing.T) {
	call := headDoCall(t, headAllowanceFixture(t, false).Func("cloned"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	cut := proveHeadAcquisitionWithin(call, pool.Within(2))
	if cut.State != proofs.EvidenceUnknown || cut.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("cut HEAD=%+v", cut)
	}
	fresh := proveHeadAcquisitionWithin(call, pool.Within(releaseSearchBudget))
	if fresh.State != proofs.EvidenceUnknown || fresh.Reason != resourceReasonHeadAcquisition {
		t.Fatalf("fresh HEAD=%+v", fresh)
	}
}

func TestHEADDefaultEffectsChildCutoff(t *testing.T) {
	call := headDoCall(t, headAllowanceFixture(t, true).Func("defaultClient"))
	assertHTTPChildCutoffFlow(t, call, func(budget *proofs.SearchBudget) resourceProof {
		return proveHeadAcquisitionWithin(call, budget)
	})
}

func headDoCall(t *testing.T, fn *ssa.Function) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssacall.CallMatchesSymbol(call.Common(), httpClientDo) {
			return call
		}
	}
	t.Fatal("missing HTTP Do")
	return nil
}

func headAllowanceFixture(t *testing.T, long bool) *ssa.Package {
	t.Helper()
	return buildHTTPAllowanceFixture(t, "headallowance", `package headallowance
 import("context";
"net/http";
"time")
 func escape(*http.Request){}
 func mark(){}
 func slow(){`, `}
 func exact(url string){r,_:=http.NewRequest("HEAD",url,nil);
c:=&http.Client{};
_,_=c.Do(r)}
 func cloned(url string){r,_:=http.NewRequestWithContext(context.Background(),"HEAD",url,nil);
r=r.WithContext(context.Background()).Clone(context.Background());
c:=&http.Client{};
_,_=c.Do(r)}
 func headers(url string){r,_:=http.NewRequest("HEAD",url,nil);
r.Header.Set("X-Key","v");
c:=&http.Client{};
_,_=c.Do(r)}
 func defaultClient(url string){r,_:=http.NewRequest("HEAD",url,nil);
_,_=http.DefaultClient.Do(r);
slow()}
 func capturedClient(url string){r,_:=http.NewRequest("HEAD",url,nil);
c:=&http.Client{};
f:=func(other *http.Request){_,_=c.Do(other)};
_,_=c.Do(r);
f(nil)}
 func modifiedCapture(url string){r,_:=http.NewRequest("HEAD",url,nil);
c:=&http.Client{};
f:=func(){c.Timeout=time.Second};
_,_=c.Do(r);
f()}
 func get(url string){r,_:=http.NewRequest("GET",url,nil);
c:=&http.Client{};
_,_=c.Do(r)}
 func phi(url string,yes bool){r,_:=http.NewRequest("HEAD",url,nil);
other,_:=http.NewRequest("HEAD",url,nil);
if yes{r=other};
c:=&http.Client{};
_,_=c.Do(r)}
 func opaque(r *http.Request){c:=&http.Client{};
_,_=c.Do(r)}
 func changed(url string){r,_:=http.NewRequest("HEAD",url,nil);
r.Method="GET";
c:=&http.Client{};
_,_=c.Do(r)}
 func escaped(url string){r,_:=http.NewRequest("HEAD",url,nil);
escape(r);
c:=&http.Client{};
_,_=c.Do(r)}
 func timeout(url string){r,_:=http.NewRequest("HEAD",url,nil);
c:=&http.Client{Timeout:time.Second};
_,_=c.Do(r)}
 func defaultModified(url string){r,_:=http.NewRequest("HEAD",url,nil);
http.DefaultClient.Timeout=time.Second;
_,_=http.DefaultClient.Do(r)}
 `, long)
}
