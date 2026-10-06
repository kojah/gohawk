package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

func TestLocalHTTPAcquisitionAllowance(t *testing.T) {
	pkg := localHTTPAllowanceFixture(t, false)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"exact", true},
		{"ownClient", true},
		{"path", true},
		{"cookie", true},
		{"helper", true},
		{"body", false},
		{"framing", false},
		{"redirect", false},
		{"changedURL", false},
		{"changedClient", false},
		{"opaque", false},
		{"otherClient", false},
		{"defaults", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := localHTTPGetCall(t, pkg.Func(test.name))
			baseline := proveLocalHeaderOnlyAcquisitionWithin(call, nil)
			if baseline.Proven() != test.want || baseline.State == proofs.EvidenceUnknown {
				t.Fatalf("default endpoint proof=%+v, want %v; SSA:\n%s", baseline, test.want, carriedSSA(t, call.Parent()))
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveLocalHeaderOnlyAcquisitionWithin(call, budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("cut endpoint proof=%+v", got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete endpoint proof=%+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("endpoint proof never completed")
		})
	}
}

func TestLocalHTTPAcquisitionChildCutoff(t *testing.T) {
	call := localHTTPGetCall(t, localHTTPAllowanceFixture(t, false).Func("exact"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	cut := proveLocalHeaderOnlyAcquisitionWithin(call, pool.Within(2))
	if cut.State != proofs.EvidenceUnknown || cut.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("endpoint cutoff=%+v", cut)
	}
	if fresh := proveLocalHeaderOnlyAcquisitionWithin(call, pool.Within(releaseSearchBudget)); !fresh.Proven() {
		t.Fatalf("fresh endpoint proof=%+v", fresh)
	}
}

func TestLocalHTTPWriterMemoFreshAllowance(t *testing.T) {
	fn := localHTTPAllowanceFixture(t, false).Func("headerHelper")
	effects := newHTTPWriterEffects()
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	child := pool.Within(2)
	if effects.headerOnly(fn.Params[0], child) || !child.Exhausted() || pool.Exhausted() {
		t.Fatal("writer cutoff must leave parent available")
	}
	fresh := pool.Within(httpEffectsBudget)
	if !effects.headerOnly(fn.Params[0], fresh) || fresh.Exhausted() {
		t.Fatal("fresh writer proof retained a cached cutoff")
	}
}

func TestLocalHTTPEffectsChildCutoffFlow(t *testing.T) {
	call := localHTTPGetCall(t, localHTTPAllowanceFixture(t, true).Func("exact"))
	assertHTTPChildCutoffFlow(t, call, func(budget *proofs.SearchBudget) resourceProof {
		return proveLocalHeaderOnlyAcquisitionWithin(call, budget)
	})
}

func localHTTPGetCall(t *testing.T, fn *ssa.Function) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssacall.CallMatchesAnySymbol(call.Common(), httpGet, httpClientGet) {
			return call
		}
	}
	t.Fatal("missing HTTP Get")
	return nil
}

func localHTTPAllowanceFixture(t *testing.T, long bool) *ssa.Package {
	t.Helper()
	return buildHTTPAllowanceFixture(t, "localhttp", `package localhttp
 import("net/http";
"net/http/httptest";
"time")
 func mark(){}
 func headers(w http.ResponseWriter,r *http.Request){`, `w.Header().Set("X-Key","v")}
 func headerHelper(w http.ResponseWriter){w.Header().Set("X-Key","v")}
 func helperHandler(w http.ResponseWriter,r *http.Request){headerHelper(w)}
 func cookieHandler(w http.ResponseWriter,r *http.Request){http.SetCookie(w,&http.Cookie{Name:"key",Value:"value"})}
 func bodyHandler(w http.ResponseWriter,r *http.Request){w.Write([]byte("body"))}
 func framingHandler(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Length","0")}
 func redirectHandler(w http.ResponseWriter,r *http.Request){w.WriteHeader(302)}
 func exact(){s:=httptest.NewServer(http.HandlerFunc(headers));
defer s.Close();
_,_=http.Get(s.URL)}
 func ownClient(){s:=httptest.NewServer(http.HandlerFunc(headers));
defer s.Close();
_,_=s.Client().Get(s.URL)}
 func path(){s:=httptest.NewServer(http.HandlerFunc(headers));
defer s.Close();
_,_=http.Get(s.URL+"/path")}
 func cookie(){s:=httptest.NewServer(http.HandlerFunc(cookieHandler));
defer s.Close();
_,_=http.Get(s.URL)}
 func helper(){s:=httptest.NewServer(http.HandlerFunc(helperHandler));
defer s.Close();
_,_=http.Get(s.URL)}
 func body(){s:=httptest.NewServer(http.HandlerFunc(bodyHandler));
defer s.Close();
_,_=http.Get(s.URL)}
 func framing(){s:=httptest.NewServer(http.HandlerFunc(framingHandler));
defer s.Close();
_,_=http.Get(s.URL)}
 func redirect(){s:=httptest.NewServer(http.HandlerFunc(redirectHandler));
defer s.Close();
_,_=http.Get(s.URL)}
 func changedURL(){s:=httptest.NewServer(http.HandlerFunc(headers));
defer s.Close();
s.URL="http://elsewhere";
_,_=http.Get(s.URL)}
 func changedClient(){s:=httptest.NewServer(http.HandlerFunc(headers));
defer s.Close();
c:=s.Client();
c.Timeout=time.Second;
_,_=c.Get(s.URL)}
 func opaque(h http.Handler){s:=httptest.NewServer(h);
defer s.Close();
_,_=http.Get(s.URL)}
 func otherClient(c *http.Client){s:=httptest.NewServer(http.HandlerFunc(headers));
defer s.Close();
_,_=c.Get(s.URL)}
 func defaults(){s:=httptest.NewServer(http.HandlerFunc(headers));
defer s.Close();
http.DefaultClient=&http.Client{};
_,_=http.Get(s.URL)}
 `, long)
}
