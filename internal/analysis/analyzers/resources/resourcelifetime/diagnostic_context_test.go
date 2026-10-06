package resourcelifetime

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

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
