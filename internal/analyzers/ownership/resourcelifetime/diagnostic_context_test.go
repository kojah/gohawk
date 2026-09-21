package resourcelifetime

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

func TestUseAfterReleaseDiagnosticContext(t *testing.T) {
	tracePath := enableResourceLifetimeContextTrace(t)
	results := analysistest.Run(t, analysistest.TestData(), Analyzer(), "resourcelifetime/useafter")
	assertUseAfterReleaseRelatedLocations(t, results)

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	assertOpaqueUseAfterReleaseTrace(t, data)
}

func assertUseAfterReleaseRelatedLocations(t *testing.T, results []*analysistest.Result) {
	t.Helper()
	foundDiagnostic := false
	for _, result := range results {
		if result.Pass == nil {
			continue
		}
		for _, diagnostic := range result.Diagnostics {
			position := result.Pass.Fset.Position(diagnostic.Pos)
			if filepath.Base(position.Filename) != "storage.go" || position.Line != 93 {
				continue
			}
			foundDiagnostic = true
			want := map[string]int{"resource acquired here": 86, "resource released here": 91}
			for _, related := range diagnostic.Related {
				line, ok := want[related.Message]
				if !ok {
					t.Errorf("unexpected related location %q", related.Message)
					continue
				}
				relatedPosition := result.Pass.Fset.Position(related.Pos)
				if filepath.Base(relatedPosition.Filename) != "storage.go" || relatedPosition.Line != line {
					t.Errorf("related location %q = %s, want storage.go:%d", related.Message, relatedPosition, line)
				}
				if related.End <= related.Pos {
					t.Errorf("related location %q has no precise range", related.Message)
				}
				delete(want, related.Message)
			}
			if len(want) != 0 {
				t.Errorf("missing related locations: %v", want)
			}
		}
	}
	if !foundDiagnostic {
		t.Fatal("missing use-after-release diagnostic at storage.go:93")
	}
}

func assertOpaqueUseAfterReleaseTrace(t *testing.T, data []byte) {
	t.Helper()
	foundOpaqueEffect := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string            `json:"reason"`
			Phase     string            `json:"phase"`
			Outcome   string            `json:"outcome"`
			Position  string            `json:"position"`
			Candidate string            `json:"candidate"`
			Details   map[string]string `json:"details"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != "release-use-opaque-effect" {
			continue
		}
		foundOpaqueEffect = true
		if event.Phase != "decision" || event.Outcome != "unknown" ||
			event.Position == "" || event.Candidate == "" || event.Details["instruction"] == "" {
			t.Errorf("unexpected opaque-effect decision: %+v", event)
		}
	}
	if !foundOpaqueEffect {
		t.Error("missing use-after-release opaque-effect decision")
	}
}

func enableResourceLifetimeContextTrace(t *testing.T) string {
	t.Helper()
	flags := flag.NewFlagSet("resource-context-trace", flag.ContinueOnError)
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
		"gohawk-trace": "resourcelifetime", "gohawk-trace-candidate": "", "gohawk-trace-file": path,
	} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	return path
}
