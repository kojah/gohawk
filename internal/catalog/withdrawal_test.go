package catalog

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestCatalogAnalyzerIdentityIncludesWithdrawnDeclarations(t *testing.T) {
	for _, test := range []struct {
		name          string
		delisted      []bool
		wantDuplicate bool
		wantAnalyzers int
	}{
		{name: "active", delisted: []bool{false}, wantAnalyzers: 1},
		{name: "withdrawn", delisted: []bool{true}},
		{name: "both active", delisted: []bool{false, false}, wantDuplicate: true},
		{name: "both withdrawn", delisted: []bool{true, true}, wantDuplicate: true},
		{name: "withdrawn then active", delisted: []bool{true, false}, wantDuplicate: true},
		{name: "active then withdrawn", delisted: []bool{false, true}, wantDuplicate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			group := GroupSpec{ID: "group", Doc: "group docs", DocPath: "group-docs"}
			for _, delisted := range test.delisted {
				group.Analyzers = append(group.Analyzers, AnalyzerSpec{
					Analyzer: &analysis.Analyzer{Name: "sample"},
					Checks: []CheckInfo{{
						ID: "sample/check", Doc: "check docs", Kind: KindDefect, Tier: TierCore, Delisted: delisted,
					}},
				})
			}
			catalog, err := NewCatalog([]GroupSpec{group}, []AnalyzerID{"sample"})
			if test.wantDuplicate {
				if err == nil || !strings.Contains(err.Error(), `analyzer "sample" is declared more than once`) {
					t.Fatalf("NewCatalog error = %v, want duplicate analyzer declaration", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := len(catalog.Analyzers()); got != test.wantAnalyzers {
				t.Fatalf("analyzer count = %d, want %d", got, test.wantAnalyzers)
			}
			if test.wantAnalyzers == 0 && len(catalog.Groups()) != 0 {
				t.Fatal("withdrawn analyzer left a selectable group")
			}
		})
	}
}
