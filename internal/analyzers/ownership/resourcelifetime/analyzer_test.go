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
		"gohawk-trace": "resourcelifetime", "gohawk-trace-candidate": "reassigned_cleanup.go", "gohawk-trace-file": path,
	} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "resourcelifetime", "resourcelifetime/useafter")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
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
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	for name, value := range map[string]string{"contracts": "http,compress", "require-reader-close": "false"} {
		if err := analyzer.Flags.Set(name, value); err != nil {
			t.Fatalf("set %s=%s: %v", name, value, err)
		}
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "resourcelifetime/config")
}

// boundedSearchDeadline is generous: the bounded analysis of the fixture below
// takes about a third of a second, and the unbounded search did not finish in
// sixty. Anything between the two means the budget stopped applying.
const boundedSearchDeadline = 15 * time.Second

// TestRecursiveReleaseSearchStaysBounded fails if the release search is once
// again unbounded on mutually recursive callees. It is a cost test, so it
// asserts elapsed time rather than diagnostics; the fixture expects none.
func TestRecursiveReleaseSearchStaysBounded(t *testing.T) {
	start := time.Now()
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "recursivecleanup")
	if elapsed := time.Since(start); elapsed > boundedSearchDeadline {
		t.Errorf("analyzing mutually recursive callees took %s, want under %s; the release search is not bounded",
			elapsed, boundedSearchDeadline)
	}
}
