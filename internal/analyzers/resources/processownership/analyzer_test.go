package processownership

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

type processTraceEvent struct {
	Reason    string `json:"reason"`
	Phase     string `json:"phase"`
	Outcome   string `json:"outcome"`
	Candidate string `json:"candidate"`
	Function  string `json:"function"`
}

func TestAnalyzer(t *testing.T) {
	path := processTraceFile(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "processownership")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event processTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase == "decision" {
			found[event.Outcome] = true
		}
		assertProcessTraceBoundary(t, event, found)
	}
	for _, outcome := range []string{
		"accepted", "rejected", "unknown", "merged-wait-proven", "helper-result", "returned-handle-owner", "immediate-process-guard",
	} {
		if !found[outcome] {
			t.Errorf("missing process trace outcome %s", outcome)
		}
	}
}

func assertProcessTraceBoundary(t *testing.T, event processTraceEvent, found map[string]bool) {
	t.Helper()
	for _, boundary := range []struct {
		name, phase, reason, outcome, file, function string
	}{
		{"helper-result", "decision", "helper-command-ownership-unknown", "unknown", "helper_results.go:", ""},
		{"merged-wait-proven", "decision", "wait-ownership-proven", "accepted", "merged_waiters.go:", ""},
		{"returned-handle-owner", "decision", "ambiguous-wait-ownership", "unknown", "returned_handles.go:", ""},
		{
			"immediate-process-guard", "evidence", "successful-start-process-non-nil", "accepted",
			"process_guards.go:", ".releaseImmediatelyGuardedMergedReturn",
		},
	} {
		if event.Phase != boundary.phase || event.Reason != boundary.reason ||
			!strings.Contains(event.Candidate, boundary.file) || !strings.HasSuffix(event.Function, boundary.function) {
			continue
		}
		if event.Outcome != boundary.outcome {
			t.Errorf("%s: want %s, got %+v", boundary.name, boundary.outcome, event)
		}
		found[boundary.name] = true
	}
}

func processTraceFile(t *testing.T) string {
	t.Helper()
	flags := flag.NewFlagSet("process-trace", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	t.Cleanup(func() {
		for name, value := range map[string]string{
			"gohawk-trace": "none", "gohawk-trace-candidate": "", "gohawk-trace-file": os.DevNull,
		} {
			if err := flags.Set(name, value); err != nil {
				t.Error(err)
			}
		}
	})
	for name, value := range map[string]string{
		"gohawk-trace": "processownership", "gohawk-trace-candidate": "", "gohawk-trace-file": path,
	} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	return path
}
