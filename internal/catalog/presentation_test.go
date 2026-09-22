package catalog

import (
	"testing"

	"github.com/kojah/gohawk/internal/check"
	"golang.org/x/tools/go/analysis"
)

func TestGroupsAlphabetizeWithoutChangingExecutionOrder(t *testing.T) {
	t.Parallel()
	var specs []AnalyzerSpec
	for _, name := range []string{"zebra", "alpha", "middle"} {
		specs = append(specs, AnalyzerSpec{
			Analyzer: &analysis.Analyzer{Name: name, Doc: name, Run: func(*analysis.Pass) (any, error) { return nil, nil }},
			Checks:   []CheckInfo{{ID: check.ID(name + "/check"), Doc: name, Kind: KindDefect, Tier: TierCore}},
		})
	}
	catalog, err := NewCatalog([]GroupSpec{{ID: "group", Doc: "Group", DocPath: "group", Analyzers: specs}},
		[]AnalyzerID{"middle", "zebra", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range []string{"alpha", "middle", "zebra"} {
		if got := catalog.Groups()[0].Analyzers[index].Analyzer.Name; got != want {
			t.Errorf("presentation[%d] = %q, want %q", index, got, want)
		}
	}
	for index, want := range []string{"middle", "zebra", "alpha"} {
		if got := catalog.Analyzers()[index].Analyzer.Name; got != want {
			t.Errorf("execution[%d] = %q, want %q", index, got, want)
		}
	}
	if specs[0].Analyzer.Name != "zebra" {
		t.Fatal("sorting changed caller-owned declarations")
	}
}
