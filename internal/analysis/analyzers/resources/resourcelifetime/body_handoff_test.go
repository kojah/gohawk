package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// Probe the boundary independently: broader ownership rules may already decline
// a mutated response, so diagnostic absence cannot prove correct Body identity.
func TestResponseBodyAggregateHandoff(t *testing.T) {
	for _, test := range []struct {
		name, body string
		unknown    bool
	}{
		{"copiedBody", `ch <- packet{body: response.Body}`, true},
		{"nestedBody", `ch <- packet{nested: inner{body: response.Body}}`, true},
		{"savedBody", `body := response.Body; response.Body = replacement; ch <- packet{body: body}`, true},
		{"savedAggregate", `p := packet{body: response.Body}; old := p; p.body = replacement; ch <- old`, true},
		{"replacedBody", `response.Body = replacement; ch <- packet{body: response.Body}`, false},
		{"replacedAggregate", `p := packet{body: response.Body}; p.body = replacement; ch <- p`, false},
		{"unrelatedResponse", `ch <- packet{body: other.Body}`, false},
		{"responseMetadata", `_ = response.Body; ch <- packet{status: response.Status}`, false},
		{"readResponseData", `data, _ = io.ReadAll(response.Body); ch <- packet{data: data}`, false},
		{"responseData", `_ = response.Body; ch <- packet{data: data}`, false},
		{"opaqueResponse", `mutate(response); ch <- packet{body: response.Body}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "bodyhandoff", `package bodyhandoff
 import "net/http"
 import "io"
 type inner struct { body io.ReadCloser }
 type packet struct { body io.ReadCloser; nested inner; status string; data []byte }
 func acquire() *http.Response
 func mutate(*http.Response)
 func probe(ch chan packet, other *http.Response, replacement io.ReadCloser, data []byte) {
  response := acquire(); _ = response; `+test.body+`
 }
`)
			function := pkg.Func("probe")
			var resource ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
				if call.Common().StaticCallee() == pkg.Func("acquire") {
					resource = call
				}
			}
			sends := ssaflow.InstructionsOf[*ssa.Send](function)
			if resource == nil || len(sends) != 1 {
				t.Fatal("missing acquisition or handoff")
			}
			analysis := resourceAnalysis{function: function, resource: resource, contract: resourceContract{family: resourceFamilyHTTP}}
			proof := analysis.responseBodyAggregateHandoff(sends[0].X, sends[0])
			want := proofs.EvidenceDisproven
			if test.unknown {
				want = proofs.EvidenceUnknown
			}
			if proof.State != want || test.unknown && proof.Reason != resourceReasonResponseBodyAggregateHandoff {
				t.Fatalf("body handoff = %+v, want state %v", proof, want)
			}
		})
	}
}
