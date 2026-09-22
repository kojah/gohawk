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

func TestAnalyzer(t *testing.T) {
	path := processTraceFile(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "processownership")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
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
		if event.Phase != "decision" {
			continue
		}
		found[event.Outcome] = true
		if event.Reason == "helper-command-ownership-unknown" && strings.Contains(event.Candidate, "helper_results.go:") {
			if event.Outcome != "unknown" {
				t.Errorf("helper command claimed exact ownership: %+v", event)
			}
			found["helper-result"] = true
		}
		if event.Reason == "wait-ownership-proven" && strings.Contains(event.Candidate, "merged_waiters.go:") {
			if event.Outcome != "accepted" {
				t.Errorf("exact successful-start merge not accepted: %+v", event)
			}
			found["merged-wait-proven"] = true
		}
	}
	for _, outcome := range []string{"accepted", "rejected", "unknown", "merged-wait-proven", "helper-result"} {
		if !found[outcome] {
			t.Errorf("missing process trace outcome %s", outcome)
		}
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
