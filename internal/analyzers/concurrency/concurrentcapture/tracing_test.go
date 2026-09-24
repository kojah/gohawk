package concurrentcapture

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

func TestCaptureLockRegionTrace(t *testing.T) {
	flags := flag.NewFlagSet("capture-trace", flag.ContinueOnError)
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
	set("gohawk-trace", "concurrentcapture")
	set("gohawk-trace-file", path)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "concurrentcapture")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
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
		if event.Phase == "decision" && strings.Contains(event.Candidate, "region_guards.go:") {
			seen[event.Reason+":"+event.Outcome] = true
		}
	}
	for _, outcome := range []string{"capture-lock-held:unknown", "capture-unguarded-write:accepted"} {
		if !seen[outcome] {
			t.Errorf("missing %s trace in region guard fixture", outcome)
		}
	}
}
