package deferinloop

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func runDeferLifetimeTrace(t *testing.T) {
	t.Helper()
	flags := flag.NewFlagSet("defer-trace", flag.ContinueOnError)
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
	set("gohawk-trace", "deferinloop")
	set("gohawk-trace-candidate", "")
	set("gohawk-trace-file", path)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "deferinloop")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"flow_merges.go:13:":   "lifetime-unknown-at-backedge:unknown",
		"result_guards.go:20:": "no-live-backedge:accepted",
		"flow_merges.go:29:":   "lifetime-unknown-at-backedge:unknown",
		"flow_merges.go:45:":   "live-at-backedge:rejected",
		"flow_merges.go:76:":   "live-at-backedge:rejected",
	}
	seen := map[string]bool{}
	candidates := map[string]int{}
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
		if event.Phase == "candidate" {
			if _, seen := candidates[event.Candidate]; !seen {
				candidates[event.Candidate] = 0
			}
		}
		if event.Phase != "decision" || event.Reason == "diagnostic-reported" {
			continue
		}
		candidates[event.Candidate]++
		for function, outcome := range want {
			if strings.Contains(event.Candidate, function) {
				seen[function] = true
				if event.Reason+":"+event.Outcome != outcome {
					t.Errorf("%s: want %s, got %+v", function, outcome, event)
				}
			}
		}
	}
	for function := range want {
		if !seen[function] {
			t.Errorf("missing lifetime decision for %s", function)
		}
	}
	for candidate, count := range candidates {
		if count != 1 {
			t.Errorf("%s: want one proof decision, got %d", candidate, count)
		}
	}
}
