package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
