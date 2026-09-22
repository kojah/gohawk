package channelsafety

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestDoubleCloseBudgetIsUnknown(t *testing.T) {
	budget := ssaflow.NewSearchBudget(0)
	proof := proveRepeatedClose(ssaflow.NewStorage(budget), budget, nil, []closeWitness{{}})
	if proof.State != ssaflow.EvidenceUnknown || proof.Reason != "double-close-budget-exhausted" {
		t.Fatalf("exhausted proof = %+v", proof)
	}
}

func TestDoubleCloseDiagnosticContext(t *testing.T) {
	path := enableChannelSafetyTrace(t)
	results := analysistest.Run(t, analysistest.TestData(), Analyzer(), "doubleclose")
	count := 0
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			count++
			if diagnostic.Category != string(check.ChannelDoubleClose) || len(diagnostic.Related) != 1 {
				t.Fatalf("unexpected diagnostic: %+v", diagnostic)
			}
			first := diagnostic.Related[0]
			if first.Message != "channel first closed here" || first.End <= first.Pos || first.Pos > diagnostic.Pos {
				t.Errorf("invalid earlier close location: %+v", first)
			}
		}
	}
	if count != 5 {
		t.Errorf("diagnostics = %d, want 5", count)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"double-close-proven": "rejected", "close-channel-identity-not-proven": "unknown"}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string `json:"reason"`
			Phase     string `json:"phase"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if outcome, ok := want[event.Reason]; ok {
			if event.Phase != "decision" || event.Outcome != outcome || event.Candidate == "" {
				t.Errorf("invalid decision: %+v", event)
			}
			delete(want, event.Reason)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing decisions: %v", want)
	}
}
