package producerlifecycle

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestReceiverTrace(t *testing.T) {
	flags := flag.NewFlagSet("producer-trace", flag.ContinueOnError)
	trace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	for name, value := range map[string]string{"gohawk-trace": "producerlifecycle", "gohawk-trace-file": path} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := flags.Set("gohawk-trace", "none"); err != nil {
			t.Error(err)
		}
	})
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "helpers")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"producer-exceeds-receives": "rejected", "producer-within-receive-count": "accepted",
		"receiver-helper-unknown": "unknown", "asynchronous-receiver": "unknown",
	}
	candidates := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Phase     string `json:"phase"`
			Reason    string `json:"reason"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase == "candidate" && event.Reason == "producer-send" {
			candidates[event.Candidate] = true
		}
		if event.Phase != "decision" || strings.HasPrefix(event.Reason, "diagnostic-") {
			continue
		}
		if !candidates[event.Candidate] {
			t.Errorf("decision without candidate: %+v", event)
		}
		delete(candidates, event.Candidate)
		if outcome, ok := want[event.Reason]; ok {
			if event.Outcome != outcome {
				t.Errorf("%s: got %s, want %s", event.Reason, event.Outcome, outcome)
			}
			delete(want, event.Reason)
		}
	}
	if len(want) != 0 || len(candidates) != 0 {
		t.Errorf("missing decisions: %v; undecided candidates: %v", want, candidates)
	}
}
