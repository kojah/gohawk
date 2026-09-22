package channelprotocol

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestTraceDecisions(t *testing.T) {
	flags := flag.NewFlagSet("protocol-trace", flag.ContinueOnError)
	trace.RegisterFlags(flags)
	set := func(name, value string) {
		t.Helper()
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		set("gohawk-trace", "none")
		set("gohawk-trace-file", os.DevNull)
	})
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	set("gohawk-trace", "channelprotocol")
	set("gohawk-trace-file", path)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "channelprotocol", "mixedcycles")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	checkTraceDecisions(t, data)
}

func checkTraceDecisions(t *testing.T, data []byte) {
	t.Helper()
	want := map[string]string{
		"protocol-wait-cycle": "rejected", "protocol-buffer-allows-progress": "accepted",
		"protocol-participants-unknown": "unknown", "protocol-summary-limit": "unknown",
		"protocol-control-flow-unknown": "unknown", "recursive-protocol": "unknown",
		"protocol-group-count-unknown": "unknown", "protocol-deferred-effects-unknown": "unknown",
		"protocol-group-scope-unknown": "unknown",
		"mutex-join-cycle":             "rejected", "mutex-channel-cycle": "rejected", "mixed-mutex-scope-unknown": "unknown",
	}
	candidates := map[string]bool{}
	decisions := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Check     string `json:"check"`
			Phase     string `json:"phase"`
			Reason    string `json:"reason"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
			Position  string `json:"position"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(event.Check, "channelprotocol/") {
			continue
		}
		key := event.Check + "|" + event.Candidate
		if event.Phase == "candidate" && event.Reason == "protocol-launch" {
			candidates[key] = true
		}
		if event.Phase != "decision" {
			continue
		}
		if !candidates[key] || decisions[key] || event.Position != event.Candidate {
			t.Errorf("invalid decision association: %+v", event)
		}
		decisions[key] = true
		if outcome, ok := want[event.Reason]; ok {
			if event.Outcome != outcome {
				t.Errorf("%s outcome = %s, want %s", event.Reason, event.Outcome, outcome)
			}
			delete(want, event.Reason)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing decisions: %v", want)
	}
}
