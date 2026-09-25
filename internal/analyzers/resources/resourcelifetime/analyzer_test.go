package resourcelifetime

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kojah/gohawk/internal/analyzertest"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	flags := flag.NewFlagSet("cleanup-trace", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	// Trace flags are process-global and do not expose a reset API. This test
	// owns their configuration; clear the candidate, select no analyzer, and
	// close the temporary destination before its directory is removed.
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
		"gohawk-trace": "resourcelifetime", "gohawk-trace-candidate": "", "gohawk-trace-file": path,
	} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	// Reuse this run for diagnostic context and trace checks: repeating the
	// fixtures also repeats dependency loading and fact serialization checks.
	results := analyzertest.Run(t, analysistest.TestData(), Analyzer(),
		"resourcelifetime", "resourcelifetime/useafter", "processexit", "processexitlib", "processexitrecursive")
	assertUseAfterReleaseRelatedLocations(t, results)
	assertMissingReleaseEvidence(t, results)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	foundReturnPath := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string `json:"reason"`
			Phase     string `json:"phase"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
			Function  string `json:"function"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason == "resource-return-path" && strings.Contains(event.Candidate, "sql_boundaries.go:") {
			foundReturnPath = true
			if event.Phase != "evidence" || event.Outcome != "observed" {
				t.Errorf("unexpected return-path evidence: %+v", event)
			}
		}
		if event.Reason != "captured-by-prior-cleanup" {
			continue
		}
		found = true
		if event.Phase != "evidence" || event.Outcome != "accepted" ||
			!strings.Contains(event.Candidate, "reassigned_cleanup.go:21:") || event.Function != "resourcelifetime.reassignedCleanup" {
			t.Errorf("unexpected prior-cleanup evidence: %+v", event)
		}
	}
	if !found {
		t.Error("missing prior-cleanup trace evidence")
	}
	if !foundReturnPath {
		t.Error("missing resource return-path evidence")
	}
	assertSQLBoundaryTrace(t, data)
	assertFollowupBoundaryTrace(t, data)
	assertUseAfterTrace(t, data)
	assertOpaqueUseAfterReleaseTrace(t, data)
}

func assertUseAfterTrace(t *testing.T, data []byte) {
	t.Helper()
	want := map[string]string{
		"release-dominates-use":         "evidence",
		"release-use-opaque-effect":     "decision",
		"release-does-not-dominate-use": "decision",
		"known-resource-direct-release": "evidence",
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string `json:"reason"`
			Phase     string `json:"phase"`
			Candidate string `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		phase, ok := want[event.Reason]
		if !ok || !strings.Contains(event.Candidate, "useafter/") {
			continue
		}
		if event.Phase != phase {
			t.Errorf("unexpected use-after trace: %+v", event)
		}
		delete(want, event.Reason)
	}
	if len(want) != 0 {
		t.Errorf("missing use-after traces: %v", want)
	}
}

func assertSQLBoundaryTrace(t *testing.T, data []byte) {
	t.Helper()
	want := map[string]string{
		"statement-parent-closed":             "evidence",
		"context-canceled-before-acquisition": "decision",
	}
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
		phase, ok := want[event.Reason]
		if !ok || !strings.Contains(event.Candidate, "sql_boundaries.go:") {
			continue
		}
		if event.Phase != phase || event.Outcome != "accepted" {
			t.Errorf("unexpected SQL boundary trace: %+v", event)
		}
		delete(want, event.Reason)
	}
	if len(want) != 0 {
		t.Errorf("missing SQL boundary traces: %v", want)
	}
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	for name, value := range map[string]string{"contracts": "http,compress", "require-memory-writer-close": "true"} {
		if err := analyzer.Flags.Set(name, value); err != nil {
			t.Fatalf("set %s=%s: %v", name, value, err)
		}
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "resourcelifetime/config")
}

// boundedSearchDeadline bounds the analyzer action, not package loading or
// prerequisite fact inference. Those costs vary independently of the release
// search, especially under race instrumentation. The formerly unbounded search
// did not finish in sixty seconds; retain a much smaller action deadline.
const boundedSearchDeadline = 15 * time.Second

// TestRecursiveReleaseSearchStaysBounded fails if the release search is once
// again unbounded on mutually recursive callees. It is a cost test, so it
// asserts elapsed time rather than diagnostics; the fixture expects none.
func TestRecursiveReleaseSearchStaysBounded(t *testing.T) {
	start := time.Now()
	results := analyzertest.Run(t, analysistest.TestData(), Analyzer(), "recursivecleanup")
	if len(results) != 1 || results[0].Action == nil || results[0].Action.Err != nil {
		t.Fatal("expected one successful analyzer action for the recursive fixture")
	}
	// checker.Action.Duration starts after prerequisite actions finish. Timing
	// the whole harness falsely attributes dependency work to recursive search.
	elapsed := results[0].Action.Duration
	t.Logf("recursive fixture: analyzer=%s, complete harness=%s", elapsed, time.Since(start))
	if elapsed > boundedSearchDeadline {
		t.Errorf("analyzing mutually recursive callees took %s, want under %s; the release search is not bounded",
			elapsed, boundedSearchDeadline)
	}
}
