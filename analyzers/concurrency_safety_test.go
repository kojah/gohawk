package analyzers

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzers/reliability/condsafety"
	"github.com/kojah/gohawk/internal/analyzers/reliability/waitgroupsafety"
	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

// Exercise the two consumers through the same tracer without duplicating the
// global flag lifecycle in each analyzer's behavioral test harness.
func TestConcurrencySafetyTrace(t *testing.T) {
	flags := flag.NewFlagSet("concurrency-safety", flag.ContinueOnError)
	trace.RegisterFlags(flags)
	set := func(name, value string) {
		t.Helper()
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { set("gohawk-trace", "none"); set("gohawk-trace-file", os.DevNull) })
	for _, analyzer := range []*analysis.Analyzer{condsafety.Analyzer(), waitgroupsafety.Analyzer()} {
		t.Run(analyzer.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			set("gohawk-trace", analyzer.Name)
			set("gohawk-trace-file", path)
			fixtures, err := filepath.Abs(filepath.Join("..", "internal", "analyzers", "reliability", analyzer.Name, "testdata"))
			if err != nil {
				t.Fatal(err)
			}
			analyzertest.Run(t, fixtures, analyzer, analyzer.Name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			assertSafetyDecisions(t, data, analyzer.Name)
		})
	}
}

func assertSafetyDecisions(t *testing.T, data []byte, analyzer string) {
	t.Helper()
	want := map[string]string{"cond-wait-unlocked": "rejected", "cond-wait-held": "accepted", "cond-locker-unknown": "unknown"}
	if analyzer == "waitgroupsafety" {
		want = map[string]string{"counter-underflow": "rejected", "counter-no-underflow": "accepted", "counter-initial-state-unknown": "unknown"}
	}
	candidates := make(map[string]bool)
	outcomes := make(map[string]bool)
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Phase     string `json:"phase"`
			Candidate string `json:"candidate"`
			Reason    string `json:"reason"`
			Outcome   string `json:"outcome"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase == "candidate" {
			candidates[event.Candidate] = true
		}
		if event.Phase != "decision" {
			continue
		}
		if event.Candidate == "" || !candidates[event.Candidate] || event.Reason == "" {
			t.Errorf("unattributed decision: %+v", event)
		}
		outcomes[event.Outcome] = true
		if expected, ok := want[event.Reason]; ok {
			if event.Outcome != expected {
				t.Errorf("%s: outcome %s, want %s", event.Reason, event.Outcome, expected)
			}
			delete(want, event.Reason)
		}
	}
	for _, outcome := range []string{"accepted", "rejected", "unknown"} {
		if !outcomes[outcome] {
			t.Errorf("missing %s decision", outcome)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing decisions: %v", want)
	}
}
