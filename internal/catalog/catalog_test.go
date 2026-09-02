package catalog

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestCatalogValidatesDeclarations(t *testing.T) {
	t.Parallel()
	valid := func() []GroupSpec {
		return []GroupSpec{{
			ID: "group", Doc: "group docs", DocPath: "group-docs",
			Analyzers: []AnalyzerSpec{{
				Analyzer: &analysis.Analyzer{Name: "sample", Doc: "sample docs", Run: func(*analysis.Pass) (any, error) { return nil, nil }},
				Checks:   []CheckInfo{{ID: "sample/check", Doc: "check docs", Kind: KindDefect, Tier: TierCore}},
			}},
		}}
	}
	tests := []struct {
		name   string
		mutate func([]GroupSpec) ([]GroupSpec, []AnalyzerID)
		want   string
	}{
		{name: "valid", mutate: func(groups []GroupSpec) ([]GroupSpec, []AnalyzerID) { return groups, []AnalyzerID{"sample"} }},
		{name: "check owner", mutate: func(groups []GroupSpec) ([]GroupSpec, []AnalyzerID) {
			groups[0].Analyzers[0].Checks[0].ID = "other/check"
			return groups, []AnalyzerID{"sample"}
		}, want: "invalid check identity"},
		{name: "missing check kind", mutate: func(groups []GroupSpec) ([]GroupSpec, []AnalyzerID) {
			groups[0].Analyzers[0].Checks[0].Kind = ""
			return groups, []AnalyzerID{"sample"}
		}, want: "invalid kind"},
		{name: "missing execution entry", mutate: func(groups []GroupSpec) ([]GroupSpec, []AnalyzerID) {
			return groups, nil
		}, want: "execution order contains 0 analyzers"},
		{name: "unknown execution entry", mutate: func(groups []GroupSpec) ([]GroupSpec, []AnalyzerID) {
			return groups, []AnalyzerID{"other"}
		}, want: "unknown analyzer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			groups, order := test.mutate(valid())
			_, err := NewCatalog(groups, order)
			if test.want == "" {
				if err != nil {
					t.Fatalf("NewCatalog: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewCatalog error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()
	analyzer := &analysis.Analyzer{Name: "sample", Doc: "sample docs", Run: func(*analysis.Pass) (any, error) { return nil, nil }}
	catalog, err := NewCatalog([]GroupSpec{{
		ID: "group", Doc: "group docs", DocPath: "group-docs",
		Analyzers: []AnalyzerSpec{{Analyzer: analyzer, Checks: []CheckInfo{{ID: "sample/check", Doc: "check docs", Kind: KindDefect, Tier: TierCore}}}},
	}}, []AnalyzerID{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	groups := catalog.Groups()
	groups[0].Analyzers[0].Checks[0].Doc = "changed"
	spec, ok := catalog.Analyzer("sample")
	if !ok {
		t.Fatal("sample analyzer missing")
	}
	if spec.Checks[0].Doc != "check docs" {
		t.Fatalf("catalog check mutated through returned groups: %q", spec.Checks[0].Doc)
	}
}
