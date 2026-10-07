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

func TestHTTPDefaultOverridePolicies(t *testing.T) {
	pkg := httpDefaultEffectsFixture(t)
	for _, test := range []struct {
		name         string
		root, strict bool
	}{
		{"ordinary", false, false},
		{"harmlessHelper", false, false},
		{"directDo", false, true},
		{"nestedDo", true, true},
		{"clientStore", true, true},
		{"transportStore", true, true},
		{"clientField", true, true},
		{"nestedStore", true, true},
		{"recursive", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			if got := !proveDefaultClientUnmodifiedWithin(fn, nil).Proven(); got != test.root {
				t.Fatalf("HEAD root override=%v, want %v; SSA:\n%s", got, test.root, carriedSSA(t, fn))
			}
			if got := newHTTPWriterEffects().overrides.Function(fn, proofs.NewSearchBudget(httpEffectsBudget)); got != test.strict {
				t.Fatalf("strict override=%v, want %v; SSA:\n%s", got, test.strict, carriedSSA(t, fn))
			}
		})
	}
}

func TestHTTPDefaultOverrideRootPolicyDoesNotChangeMemo(t *testing.T) {
	fn := httpDefaultEffectsFixture(t).Func("directDo")
	effects := newHTTPWriterEffects()
	if !effects.overrides.Function(fn, proofs.NewSearchBudget(httpEffectsBudget)) {
		t.Fatal("strict default-effect summary lost the default-client load")
	}
	if effects.scanDefaultOverrides(fn, proofs.NewSearchBudget(httpEffectsBudget), true) {
		t.Fatal("cached strict summary intercepted the root-only Do allowance")
	}
	if !effects.overrides.Function(fn, proofs.NewSearchBudget(httpEffectsBudget)) {
		t.Fatal("root-only allowance replaced the strict declaration summary")
	}
}

func TestHTTPDefaultOverrideAllowance(t *testing.T) {
	fn := httpDefaultEffectsFixture(t).Func("harmlessHelper")
	for _, root := range []bool{false, true} {
		completed := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			budget := proofs.NewSearchBudget(limit)
			got := newHTTPWriterEffects().scanDefaultOverrides(fn, budget, root)
			if resourceFlowExhausted(budget) {
				if !got {
					t.Fatalf("interrupted default-effect scan established no override at %d", limit)
				}
				continue
			}
			if got {
				t.Fatal("complete harmless helper established an override")
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("default-effect scan never completed")
		}
	}
}

func TestHTTPDefaultOverrideFreshMemo(t *testing.T) {
	fn := httpDefaultEffectsFixture(t).Func("harmlessHelper")
	effects := newHTTPWriterEffects()
	pool := proofs.NewSearchBudget(httpEffectsBudget)
	child := pool.Within(2)
	if !effects.overrides.Function(fn, child) || !child.Exhausted() || pool.Exhausted() {
		t.Fatal("child cutoff must retain possible modification and parent availability")
	}
	fresh := pool.Within(httpEffectsBudget)
	if effects.overrides.Function(fn, fresh) || fresh.Exhausted() {
		t.Fatal("fresh allowance must recover harmless effects without cached cutoff")
	}
}

func httpDefaultEffectsFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "httpdefaults", `package httpdefaults
 import "net/http"
 func mark(){}
 func ordinary(){mark()}
 func harmlessHelper(){ordinary()}
 func directDo(request *http.Request){_,_=http.DefaultClient.Do(request)}
 func nestedDo(request *http.Request){directDo(request)}
 func clientStore(){http.DefaultClient=&http.Client{}}
 func transportStore(transport http.RoundTripper){http.DefaultTransport=transport}
 func clientField(){http.DefaultClient.Timeout=0}
 func nestedStore(){clientStore()}
 func recursive(done bool){if done{return};recursive(done)}
 `)
}

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

// HTTP allowance controls exercise the same cutoff boundary for HEAD and
// local header-only acquisitions while keeping their protocol fixtures separate.
func assertHTTPChildCutoffFlow(t *testing.T, call *ssa.Call, prove func(*proofs.SearchBudget) resourceProof) {
	t.Helper()
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	proof := prove(pool.Within(releaseSearchBudget))
	if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Errorf("HTTP child availability=%+v", proof)
	}
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	var resource ssa.Value
	for _, extract := range ssaflow.InstructionsOf[*ssa.Extract](call.Parent()) {
		if extract.Tuple == call && extract.Index == 0 {
			resource = extract
		}
	}
	if resource == nil {
		t.Fatal("missing HTTP response result")
	}
	got := evaluateResourceFlow(nil, evidence, call, resource, resourceContract{
		family: resourceFamilyHTTP, packagePath: "net/http", cleanup: []string{"Close"},
	})
	if got.state != proofs.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil {
		t.Fatalf("HTTP cutoff fell through to leak=%+v", got)
	}
}

func buildHTTPAllowanceFixture(t *testing.T, path, prefix, suffix string, oversized bool) *ssa.Package {
	t.Helper()
	var source strings.Builder
	source.WriteString(prefix)
	if oversized {
		for range httpEffectsBudget + 10 {
			source.WriteString("mark();")
		}
	}
	source.WriteString(suffix)
	return ssaflowtest.BuildPackage(t, path, source.String())
}
