package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// These assertions test the boundary itself: ordinary ownership classification
// can already decline replaced/escaped cells, so diagnostic absence alone would
// not establish that this narrower rule rejects a stale capture.
func TestCapturedBodyCleanupRejectsStaleOrigins(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/capturetest", `package capturetest
import "net/http"
var responses map[string]*http.Response
func acquire() *http.Response { return nil }
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
`)
	for _, name := range []string{"stable", "replaced", "opaqueCell", "opaqueOwner", "mapEscape"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var resource ssa.Value
			var invocation *ssa.Call
			var closure *ssa.MakeClosure
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
				if call.Common().StaticCallee() == pkg.Func("acquire") {
					resource = call
				}
				if created, ok := call.Common().Value.(*ssa.MakeClosure); ok {
					invocation, closure = call, created
				}
			}
			if resource == nil || invocation == nil || closure == nil {
				t.Fatal("missing captured cleanup SSA")
			}
			analysis := resourceAnalysis{function: function, resource: resource, contract: resourceContract{family: "http"}}
			proof := analysis.guardedCapturedBodyCleanup(invocation, closure)
			want := ssaflow.EvidenceDisproven
			if name == "stable" {
				want = ssaflow.EvidenceUnknown
			}
			if proof.State != want {
				t.Fatalf("captured cleanup = %+v, want state %v", proof, want)
			}
		})
	}
}
