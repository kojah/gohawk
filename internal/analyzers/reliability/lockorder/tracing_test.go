package lockorder

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestLockTraceBoundaries(t *testing.T) {
	flags := flag.NewFlagSet("cycle-trace", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	set := func(name, value string) {
		t.Helper()
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	t.Cleanup(func() {
		set("gohawk-trace", "none")
		set("gohawk-trace-candidate", "")
		set("gohawk-trace-file", os.DevNull)
	})
	set("gohawk-trace", "lockorder")
	set("gohawk-trace-candidate", "")
	set("gohawk-trace-file", path)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "lockorder")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	foundUnknown := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Phase     string            `json:"phase"`
			Reason    string            `json:"reason"`
			Outcome   string            `json:"outcome"`
			Candidate string            `json:"candidate"`
			Position  string            `json:"position"`
			Details   map[string]string `json:"details"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason == "optional-loaded-mutex-unknown" && strings.Contains(event.Candidate, "optional_owners.go:") {
			foundUnknown = true
			if event.Phase != "decision" || event.Outcome != "unknown" || event.Position != event.Candidate {
				t.Errorf("invalid optional mutex uncertainty: %+v", event)
			}
			continue
		}
		if event.Reason != "opposite-order-recorded" || !strings.Contains(event.Candidate, "lock_classes.go:") {
			continue
		}
		found = true
		if event.Phase != "evidence" || event.Outcome != "rejected" ||
			event.Position == "" || event.Position == event.Candidate ||
			event.Details["held"] == "" || event.Details["acquired"] == "" {
			t.Errorf("invalid cycle evidence: %+v", event)
		}
	}
	if !found {
		t.Error("missing opposite-order evidence")
	}
	if !foundUnknown {
		t.Error("missing optional mutex uncertainty")
	}
}
