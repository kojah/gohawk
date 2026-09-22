package goroutineownership

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestConcurrencyJoinProofs(t *testing.T) {
	tracePath := enableSummaryJoinTrace(t)
	want := map[string]GoroutineOutcome{
		"importedReceive":     GoroutineLifecycleHonored,
		"importedWait":        GoroutineLifecycleHonored,
		"deferredReceive":     GoroutineLifecycleHonored,
		"branchAwareJoin":     GoroutineLifecycleHonored,
		"opaqueReceive":       GoroutineUnknown,
		"conditionalReceive":  GoroutineUnknown,
		"asynchronousReceive": GoroutineUnknown,
		"differentSignal":     GoroutineLifecycleViolated,
		"differentArgument":   GoroutineUnknown,
		"missingPath":         GoroutineLifecycleViolated,
	}
	for _, result := range analysistest.Run(t, analysistest.TestData(), Analyzer(), "summaryjoins") {
		functions, err := ssaflow.SourceSSAFunctions(result.Pass)
		if err != nil {
			t.Fatal(err)
		}
		for _, function := range functions {
			expected, ok := want[function.Name()]
			if !ok {
				continue
			}
			for _, spawn := range ssaflow.InstructionsOf[*ssa.Go](function) {
				// The first launch owns the completion signal; a later waiter is
				// deliberately not a join by the launching goroutine itself.
				analysis := newSpawnAnalysis(result.Pass, function, spawn, goroutineOwnershipConfig{mode: goroutineModeJoin})
				proof := analysis.prove()
				if proof.Outcome != expected {
					t.Errorf("%s: got %+v, want outcome %v", function.Name(), proof, expected)
				}
				delete(want, function.Name())
				break
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing proof cases: %v", want)
	}
	assertSummaryJoinTrace(t, tracePath)
}

func enableSummaryJoinTrace(t *testing.T) string {
	t.Helper()
	flags := flag.NewFlagSet("summary-joins", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	for name, value := range map[string]string{"gohawk-trace": "goroutineownership", "gohawk-trace-file": path} {
		previous := flags.Lookup(name).Value.String()
		if previous == "" {
			previous = "none"
			if name == "gohawk-trace-file" {
				previous = os.DevNull
			}
		}
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := flags.Set(name, previous); err != nil {
				t.Error(err)
			}
		})
	}
	return path
}

func assertSummaryJoinTrace(t *testing.T, path string) {
	t.Helper()
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
			Position  string `json:"position"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != "concurrency-summary-join" {
			continue
		}
		found = true
		if event.Phase != "evidence" || event.Outcome != "accepted" || event.Candidate == "" || event.Position == "" {
			t.Errorf("invalid summary join event: %+v", event)
		}
	}
	if !found {
		t.Error("missing concurrency-summary-join evidence")
	}
}
