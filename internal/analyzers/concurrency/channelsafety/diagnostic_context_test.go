package channelsafety

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestSendAfterCloseDiagnosticContext(t *testing.T) {
	tracePath := enableChannelSafetyTrace(t)
	results := analysistest.Run(t, analysistest.TestData(), Analyzer(), "channelsafety", "summaryeffects")
	assertSendAfterCloseRelatedLocation(t, results)
	assertSummaryDiagnosticContext(t, results)

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	assertChannelIdentityTrace(t, data)
}

func TestChannelCycleTraceReasons(t *testing.T) {
	tracePath := enableChannelSafetyTrace(t)
	analysistest.Run(t, analysistest.TestData(), Analyzer(), "channelcycle")

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"channel-cycle-proven":                 "accepted",
		"channel-cycle-launch-unknown":         "unknown",
		"channel-cycle-fresh-channels-unknown": "unknown",
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
		outcome, ok := want[event.Reason]
		if !ok {
			continue
		}
		if event.Phase != "decision" || event.Outcome != outcome || event.Candidate == "" {
			t.Errorf("unexpected channel cycle decision: %+v", event)
		}
		delete(want, event.Reason)
	}
	if len(want) != 0 {
		t.Errorf("missing channel cycle decisions: %v", want)
	}
}

func assertSummaryDiagnosticContext(t *testing.T, results []*analysistest.Result) {
	t.Helper()
	count := 0
	for _, result := range results {
		if result.Pass == nil || result.Pass.Pkg.Path() != "summaryeffects" {
			continue
		}
		for _, diagnostic := range result.Diagnostics {
			count++
			if len(diagnostic.Related) != 1 {
				t.Fatalf("summary diagnostic has %d related locations, want 1", len(diagnostic.Related))
			}
			send := result.Pass.Fset.Position(diagnostic.Pos)
			close := result.Pass.Fset.Position(diagnostic.Related[0].Pos)
			if filepath.Base(send.Filename) != "diagnostics.go" || close.Filename != send.Filename || close.Line != send.Line-1 {
				t.Errorf("summary positions = send %s, close %s; want adjacent caller locations", send, close)
			}
		}
	}
	if count != 5 {
		t.Errorf("summary diagnostics = %d, want 5", count)
	}
}

func assertSendAfterCloseRelatedLocation(t *testing.T, results []*analysistest.Result) {
	t.Helper()
	foundDiagnostic := false
	for _, result := range results {
		if result.Pass == nil {
			continue
		}
		for _, diagnostic := range result.Diagnostics {
			position := result.Pass.Fset.Position(diagnostic.Pos)
			if filepath.Base(position.Filename) != "storage_snapshots.go" || position.Line != 7 {
				continue
			}
			foundDiagnostic = true
			if len(diagnostic.Related) != 1 {
				t.Fatalf("related locations = %d, want 1", len(diagnostic.Related))
			}
			related := diagnostic.Related[0]
			relatedPosition := result.Pass.Fset.Position(related.Pos)
			if related.Message != "channel closed here" || filepath.Base(relatedPosition.Filename) != "storage_snapshots.go" || relatedPosition.Line != 6 {
				t.Errorf("related location = %q at %s, want close at storage_snapshots.go:6", related.Message, relatedPosition)
			}
			if related.End <= related.Pos {
				t.Errorf("related close location has no precise range: %+v", related)
			}
		}
	}
	if !foundDiagnostic {
		t.Fatal("missing send-after-close diagnostic at storage_snapshots.go:7")
	}
}

func assertChannelIdentityTrace(t *testing.T, data []byte) {
	t.Helper()
	want := map[string]string{
		"send-after-close-proven":          "rejected",
		"send-channel-identity-not-proven": "unknown",
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string            `json:"reason"`
			Phase     string            `json:"phase"`
			Outcome   string            `json:"outcome"`
			Candidate string            `json:"candidate"`
			Details   map[string]string `json:"details"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		outcome, ok := want[event.Reason]
		if !ok {
			continue
		}
		if event.Phase != "decision" || event.Outcome != outcome || event.Candidate == "" ||
			event.Details["close"] == "" || event.Details["identity_reason"] == "" {
			t.Errorf("unexpected channel identity decision: %+v", event)
		}
		delete(want, event.Reason)
	}
	if len(want) != 0 {
		t.Errorf("missing channel identity decisions: %v", want)
	}
}

func enableChannelSafetyTrace(t *testing.T) string {
	t.Helper()
	flags := flag.NewFlagSet("channelsafety-trace", flag.ContinueOnError)
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
		"gohawk-trace": "channelsafety", "gohawk-trace-candidate": "", "gohawk-trace-file": path,
	} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	return path
}
