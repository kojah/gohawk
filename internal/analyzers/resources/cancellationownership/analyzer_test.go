package cancellationownership

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
	tracePath := enableTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "cancellationownership")
	assertLabelTrace(t, tracePath)
}

func TestDiagnosticOnly(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "cancellationownership/diagnostic")
}

// enableTrace sends this analyzer's trace to a temporary file and restores
// the process-wide trace flags when the test ends.
func enableTrace(t *testing.T) string {
	t.Helper()
	flags := flag.NewFlagSet("labels", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	for name, value := range map[string]string{"gohawk-trace": "cancellationownership", "gohawk-trace-file": path} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for name, value := range map[string]string{"gohawk-trace": "none", "gohawk-trace-file": os.DevNull} {
			if err := flags.Set(name, value); err != nil {
				t.Error(err)
			}
		}
	})
	return path
}

// assertLabelTrace checks the classifier's labels: a deferred cancel and an
// imported helper whose summary calls cancel, unconditionally or in the case
// a constant selects, are releases; a variable flag, a call on another
// goroutine, and a call of a different argument stay unknown.
func assertLabelTrace(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"cancellationownership.go:30:2":  {"release", "accepted"},
		"cancellationownership.go:15:24": {"summary-release", "accepted"},
		"cancellationownership.go:20:29": {"helper-completion-unknown", "unknown"},
		"argument_cases.go:48:29":        {"summary-release", "accepted"},
		"argument_cases.go:53:29":        {"helper-completion-unknown", "unknown"},
		"argument_cases.go:59:29":        {"helper-completion-unknown", "unknown"},
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Phase    string `json:"phase"`
			Reason   string `json:"reason"`
			Outcome  string `json:"outcome"`
			Position string `json:"position"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase != "label" {
			continue
		}
		for position, expected := range want {
			if strings.HasSuffix(event.Position, position) && event.Reason == expected[0] && event.Outcome == expected[1] {
				delete(want, position)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing label steps: %v", want)
	}
}
