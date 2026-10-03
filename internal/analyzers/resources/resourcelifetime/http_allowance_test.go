package resourcelifetime

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// HTTP allowance controls exercise the same cutoff boundary for HEAD and
// local header-only acquisitions while keeping their protocol fixtures separate.
func assertHTTPChildCutoffFlow(t *testing.T, call *ssa.Call, prove func(*ssaflow.SearchBudget) resourceProof) {
	t.Helper()
	pool := ssaflow.NewSearchBudget(resourcePoolBudget)
	proof := prove(pool.Within(releaseSearchBudget))
	if proof.State != ssaflow.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
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
	if got.state != ssaflow.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil {
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
