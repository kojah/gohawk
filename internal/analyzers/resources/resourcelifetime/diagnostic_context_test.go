package resourcelifetime

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

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

// assertMissingReleaseEvidence checks that a missing-release diagnostic cites
// the leaking return, labeled with the resource's variable, and the condition
// that decides the path, and that a function without a return cites its
// closing brace.
func assertMissingReleaseEvidence(t *testing.T, results []*analysistest.Result) {
	t.Helper()
	want := map[int][]string{
		13: {"17:when this is true", "18:returns here without releasing `config`"},
		24: {"25:reaches the end of the function without releasing the resource"},
		30: {"35:returns here without releasing `config`"},
	}
	for _, result := range results {
		if result.Pass == nil {
			continue
		}
		for _, diagnostic := range result.Diagnostics {
			position := result.Pass.Fset.Position(diagnostic.Pos)
			expected, ok := want[position.Line]
			if filepath.Base(position.Filename) != "missing_release_evidence.go" || !ok {
				continue
			}
			delete(want, position.Line)
			var got []string
			for _, related := range diagnostic.Related {
				got = append(got, fmt.Sprintf("%d:%s", result.Pass.Fset.Position(related.Pos).Line, related.Message))
			}
			if !slices.Equal(got, expected) {
				t.Errorf("line %d: evidence = %q, want %q", position.Line, got, expected)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing evidence diagnostics at lines %v", want)
	}
}
