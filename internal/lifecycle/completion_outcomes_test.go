package lifecycle

import (
	"go/token"
	"reflect"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

const completionOutcomeFixture = `package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func missing(r *resource) {}
func opaque(r *resource)
func recursive(r *resource) { recursive(r) }
func cyclic(r *resource, again bool) { for again { r.Close() } }
func mixed(r *resource, again bool) { for again { r.Close() }; recursive(r) }
func callMissing(r *resource) { missing(r) }
func callOpaque(r *resource) { opaque(r) }
func callRecursive(r *resource) { recursive(r) }
func callCyclic(r *resource, again bool) { cyclic(r, again) }
func callMixed(r *resource, again bool) { mixed(r, again) }
`

// Actual searches combine unknown causes: a cycle can also contain recursive
// work, and an unavailable search can exhaust before it ever visits a body.
// The final give-up must retain the authoritative reason and provenance.
func TestCompletionUnknownReasonPriority(t *testing.T) {
	pkg := buildTestSSA(t, completionOutcomeFixture)
	for _, test := range []struct {
		name   string
		limit  int
		state  ssaflow.EvidenceState
		reason ssaflow.EvidenceReason
		local  bool
	}{
		{"callMissing", ssaflow.QueryBudget, ssaflow.EvidenceDisproven, ssaflow.EvidenceNotFound, true},
		{"callOpaque", ssaflow.QueryBudget, ssaflow.EvidenceUnknown, ssaflow.EvidenceUnavailable, false},
		{"callRecursive", ssaflow.QueryBudget, ssaflow.EvidenceUnknown, ssaflow.EvidenceUnavailable, true},
		{"callCyclic", ssaflow.QueryBudget, ssaflow.EvidenceUnknown, ssaflow.EvidenceCompletionInCycle, true},
		{"callMixed", ssaflow.QueryBudget, ssaflow.EvidenceUnknown, ssaflow.EvidenceCompletionInCycle, true},
		{"callMissing", 1, ssaflow.EvidenceUnknown, ssaflow.EvidenceBudgetExhausted, true},
		{"callMixed", 1, ssaflow.EvidenceUnknown, ssaflow.EvidenceBudgetExhausted, true},
		{"callOpaque", 0, ssaflow.EvidenceUnknown, ssaflow.EvidenceUnavailable, false},
	} {
		fn := pkg.Func(test.name)
		instruction := findLaunch(t, fn)
		finalObservations := 0
		var reason string
		var position token.Pos
		var details map[string]string
		budget := ssaflow.NewSearchBudget(test.limit).Observed(func(code string, at token.Pos, data map[string]string) {
			reason, position, details = code, at, data
			if at == instruction.Pos() && data["instruction"] == instruction.String() && data["target"] == "r" && data["methods"] == "Close" {
				finalObservations++
			}
		})
		proof := ProveCompletion(CompletionRequest{Instruction: instruction, Target: fn.Params[0], Methods: []string{"Close"}, Budget: budget})
		provenance := ssaflow.EvidenceProvenance("")
		if test.local {
			provenance = ssaflow.EvidenceFromLocalSSA
		}
		want := ssaflow.Proof{State: test.state, Reason: test.reason, Provenance: provenance}
		if proof.Proof != want || proof.PathKnown || proof.Path != "" {
			t.Errorf("%s(limit=%d): got %+v, want %+v", test.name, test.limit, proof, want)
		}
		wantDetails := map[string]string{"instruction": instruction.String(), "target": "r", "methods": "Close"}
		if finalObservations != 1 || reason != want.Reason.String() || position != instruction.Pos() || !reflect.DeepEqual(details, wantDetails) {
			t.Errorf("%s(limit=%d): final observation %s at %d %+v", test.name, test.limit, reason, position, details)
		}
	}
}
