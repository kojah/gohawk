package nilargument

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

// A reported nil proof names the earlier calls the graph did not
// summarize, bound to the same candidate as its decision.
func TestNilProofTracesUnsummarizedCalls(t *testing.T) {
	data := recordTrace(t, "tracing")
	type event struct {
		Phase     string            `json:"phase"`
		Reason    string            `json:"reason"`
		Outcome   string            `json:"outcome"`
		Candidate string            `json:"candidate"`
		Details   map[string]string `json:"details"`
	}
	var unsummarized, counts, decision *event
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var parsed event
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Fatal(err)
		}
		switch {
		case parsed.Phase == "evidence" && parsed.Reason == "earlier-call-unsummarized":
			unsummarized = &parsed
		case parsed.Phase == "evidence" && parsed.Reason == "earlier-calls":
			counts = &parsed
		case parsed.Phase == "decision" && parsed.Reason == "nil-slot-dereferenced":
			decision = &parsed
		}
	}
	if unsummarized == nil || counts == nil || decision == nil {
		t.Fatalf("missing events: unsummarized=%v counts=%v decision=%v\n%s", unsummarized, counts, decision, data)
	}
	if unsummarized.Details["reason"] != "interface-call" || counts.Details["unsummarized"] != "1" {
		t.Errorf("unsummarized call %v, counts %v", unsummarized.Details, counts.Details)
	}
	if counts.Details["heap-cached"] != "true" || counts.Details["heap-build-reason"] != "graph-build-complete" {
		t.Errorf("missing graph provenance: %v", counts.Details)
	}
	if decision.Outcome != "rejected" || unsummarized.Candidate != decision.Candidate || counts.Candidate != decision.Candidate {
		t.Errorf("events not bound to the rejected candidate: %+v %+v %+v", unsummarized, counts, decision)
	}
}

// recordTrace runs the analyzer over one fixture package with nilargument
// tracing on and returns the trace lines.
func recordTrace(t *testing.T, pkg string) []byte {
	t.Helper()
	flags := flag.NewFlagSet("nilargument-trace", flag.ContinueOnError)
	trace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	for name, value := range map[string]string{"gohawk-trace": "nilargument", "gohawk-trace-file": path} {
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
	return data
}
