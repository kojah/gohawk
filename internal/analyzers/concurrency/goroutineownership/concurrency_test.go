package goroutineownership

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
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
		"returnedWaiter":      GoroutineLifecycleHonored,
		"otherReturnedWaiter": GoroutineLifecycleViolated,
		"uninvokedWaiter":     GoroutineUnknown,
		"asynchronousWaiter":  GoroutineUnknown,
	}
	assertSpawnProofs(t, want, "summaryjoins")
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
