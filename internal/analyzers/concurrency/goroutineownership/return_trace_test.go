package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestReturnedOwnershipTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "returnlabels")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	labels := 0
	decisions := map[string]int{}
	var labelCandidate, decisionCandidate string
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(event.Function, "returnlabels.")
		if event.Phase == "label" && strings.HasPrefix(event.Details["instruction"], "return ") {
			labels++
			labelCandidate = event.Candidate
			assertReturnedTransferLabel(t, event)
		}
		if event.Phase == "decision" {
			decisions[name]++
			wantReason, wantOutcome := "unowned-return", "rejected"
			if name == "mergedReturn" {
				wantReason, wantOutcome = "join-proven", "accepted"
				decisionCandidate = event.Candidate
			}
			if event.Reason != wantReason || event.Outcome != wantOutcome || event.Candidate != event.Position {
				t.Errorf("invalid returned ownership decision: %+v", event)
			}
		}
	}
	if labels != 1 || decisions["mergedReturn"] != 1 || decisions["unrelatedReturn"] != 1 {
		t.Errorf("got %d return labels and decisions %v, want one transfer label and one decision per candidate", labels, decisions)
	}
	if labelCandidate == "" || labelCandidate != decisionCandidate {
		t.Errorf("return label candidate %q differs from final decision candidate %q", labelCandidate, decisionCandidate)
	}
}

func assertReturnedTransferLabel(t *testing.T, event followupTraceEvent) {
	t.Helper()
	if event.Function != "returnlabels.mergedReturn" || event.Reason != "returned-tracked-value" || event.Outcome != "accepted" ||
		event.Details["label"] != "transfer" || event.Candidate == "" {
		t.Errorf("invalid returned ownership label: %+v", event)
	}
}
