package trace

import (
	"bytes"
	"encoding/json"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestProbePhaseLabels(t *testing.T) {
	resetTrace(t)
	var output bytes.Buffer
	global.config.writer = &output
	global.config.selectors = map[string]bool{"example": true}
	global.active.Store(true)
	pass := &analysis.Pass{Fset: token.NewFileSet()}
	probe := For(pass, "example", "", token.NoPos)
	for _, test := range []struct {
		emit  func(Probe, Step)
		label string
	}{
		{Probe.Evidence, "evidence"},
		{Probe.Decision, "decision"},
		{Probe.Label, "label"},
		{Probe.Candidate, "candidate"},
		{Probe.Considered, "considered"},
	} {
		output.Reset()
		test.emit(probe, Step{Reason: "test", Outcome: OutcomeUnknown})
		var record Record
		if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
			t.Fatal(err)
		}
		if record.Phase != test.label || record.Outcome != OutcomeUnknown || record.Reason != "test" || strings.Count(output.String(), "\n") != 1 {
			t.Fatalf("serialized event=%s", output.String())
		}
	}
}
