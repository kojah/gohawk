package cancellationownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestProgramEntryBoundaries(t *testing.T) {
	path := enableTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "entrycontext", "callableentry", "namedentry")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decisions := map[string][2]string{
		"entrycontext/main.go:19:35": {"ambiguous-cancellation-use", "unknown"},
		"entrycontext/main.go:25:50": {"ambiguous-cancellation-use", "unknown"},
		"entrycontext/main.go:29:52": {"exact-cancellation-release", "accepted"},
		"entrycontext/main.go:34:36": {"unowned-return", "rejected"},
		"entrycontext/main.go:48:35": {"unowned-return", "rejected"},
		"callableentry/main.go:8:35": {"unowned-return", "rejected"},
		"namedentry/main.go:7:35":    {"unowned-return", "rejected"},
	}
	labels := map[string]int{
		"entrycontext/main.go:19:35": 0, "entrycontext/main.go:25:50": 0, "entrycontext/main.go:29:52": 0,
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Phase     string `json:"phase"`
			Reason    string `json:"reason"`
			Outcome   string `json:"outcome"`
			Position  string `json:"position"`
			Candidate string `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		for candidate, expected := range decisions {
			if event.Phase == "decision" && strings.HasSuffix(event.Candidate, candidate) {
				if event.Position != event.Candidate || event.Reason != expected[0] || event.Outcome != expected[1] {
					t.Errorf("%s: unexpected final decision %+v", candidate, event)
				}
				delete(decisions, candidate)
			}
		}
		if event.Phase == "label" && event.Reason == "process-lifetime-context" {
			for candidate := range labels {
				if strings.HasSuffix(event.Candidate, candidate) {
					if event.Outcome != "unknown" || !strings.HasSuffix(event.Position, "entrycontext/main.go:51:2") {
						t.Errorf("%s: unexpected process-lifetime label %+v", candidate, event)
					}
					labels[candidate]++
				}
			}
		}
	}
	if len(decisions) != 0 {
		t.Errorf("missing decisions: %v", decisions)
	}
	for candidate, count := range labels {
		if count != 1 {
			t.Errorf("%s: got %d process-lifetime labels, want one", candidate, count)
		}
	}
}
