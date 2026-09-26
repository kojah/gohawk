package producerlifecycle

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestReceiverTrace(t *testing.T) {
	events := producerTrace(t, "helpers")
	want := map[string]string{
		"producer-exceeds-receives": "rejected", "producer-within-receive-count": "accepted",
		"receiver-helper-unknown": "unknown", "asynchronous-receiver": "unknown",
	}
	candidates := map[string]bool{}
	for _, event := range events {
		if event.Phase == "candidate" && (event.Reason == "producer-send" || event.Reason == "one-shot-worker" || event.Reason == "local-channel") {
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

// A producer started by a go statement inside a loop has no provable count,
// even though the goroutine body sends once.
func TestLoopSpawnedProducerCountIsUnknown(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(analysistest.TestData(), "src", "producerlifecycle", "uncertain_counts.go"))
	if err != nil {
		t.Fatal(err)
	}
	var line int
	for index, text := range strings.Split(string(source), "\n") {
		if strings.Contains(text, "// producer started per item") {
			line = index + 1
		}
	}
	found := false
	for _, event := range producerTrace(t, "producerlifecycle") {
		if event.Phase != "decision" || !strings.Contains(event.Candidate, "uncertain_counts.go:"+strconv.Itoa(line)+":") {
			continue
		}
		found = true
		if event.Reason != "producer-count-unknown" {
			t.Errorf("loop-spawned producer decided by %s, want producer-count-unknown", event.Reason)
		}
	}
	if line == 0 || !found {
		t.Fatalf("no decision for the loop-spawned producer at line %d", line)
	}
}

type producerTraceEvent struct {
	Phase     string `json:"phase"`
	Reason    string `json:"reason"`
	Outcome   string `json:"outcome"`
	Candidate string `json:"candidate"`
}

// producerTrace runs the analyzer on a fixture package with tracing on and
// returns the trace events.
func producerTrace(t *testing.T, pkg string) []producerTraceEvent {
	t.Helper()
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
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), pkg)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var events []producerTraceEvent
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event producerTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}
