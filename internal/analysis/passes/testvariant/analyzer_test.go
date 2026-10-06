package testvariant

import (
	"reflect"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestAnalyzerProducesCanonicalTestVariant(t *testing.T) {
	result, err := Analyzer.Run(&analysis.Pass{})
	if err != nil {
		t.Fatal(err)
	}
	if resultType := reflect.TypeOf(result); resultType != Analyzer.ResultType {
		t.Fatalf("result type = %v, want %v", resultType, Analyzer.ResultType)
	}
}

func TestIncludeProductionFilesAddsMarkerPrerequisite(t *testing.T) {
	baseRequirement := &analysis.Analyzer{Name: "base", Doc: "base prerequisite", Run: func(*analysis.Pass) (any, error) { return nil, nil }}
	analyzer := &analysis.Analyzer{Name: "target", Doc: "target analyzer", Requires: []*analysis.Analyzer{baseRequirement}}

	wrapper := IncludeProductionFiles(analyzer)

	if wrapper == analyzer {
		t.Fatal("IncludeProductionFiles returned the original analyzer")
	}
	if !slices.Contains(wrapper.Requires, baseRequirement) || !slices.Contains(wrapper.Requires, Analyzer) {
		t.Fatalf("wrapper requirements = %v, want original requirement and test-variant marker", wrapper.Requires)
	}
	if len(analyzer.Requires) != 1 || analyzer.Requires[0] != baseRequirement {
		t.Fatalf("original requirements changed: %v", analyzer.Requires)
	}
}
