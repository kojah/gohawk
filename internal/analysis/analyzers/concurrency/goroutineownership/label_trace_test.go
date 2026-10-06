package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// assertClassifierLabels checks a normal analyzer run, where each selected
// instruction must be labelled once with its reason and proof strength.
func assertClassifierLabels(t *testing.T, path string, want map[string][3]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		expected, ok := want[event.Function]
		if !ok || event.Phase != "label" {
			continue
		}
		counts[event.Function]++
		if event.Reason != expected[0] || event.Outcome != expected[1] || event.Details["label"] != expected[2] ||
			event.Candidate == "" || event.Position == "" {
			t.Errorf("invalid classifier label: %+v", event)
		}
	}
	for name := range want {
		if counts[name] != 1 {
			t.Errorf("%s: got %d labels, want one", name, counts[name])
		}
	}
}
