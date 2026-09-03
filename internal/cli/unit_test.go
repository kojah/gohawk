package cli

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

type unitFact struct{}

func (unitFact) AFact() {}

func TestUnitAnalyzersKeepsFactProducerClosureOnFactsOnlyUnits(t *testing.T) {
	// Mirror the real shape: a fact producer reached only as a prerequisite of
	// a consumer, both built on buildssa.
	producer := &analysis.Analyzer{Name: "facts", Requires: []*analysis.Analyzer{buildssa.Analyzer}, FactTypes: []analysis.Fact{unitFact{}}}
	consumer := &analysis.Analyzer{Name: "consumer", Requires: []*analysis.Analyzer{buildssa.Analyzer, producer}}
	other := &analysis.Analyzer{Name: "other", Requires: []*analysis.Analyzer{buildssa.Analyzer}}
	selected := []*analysis.Analyzer{consumer, other}

	// A dependency unit: VetxOnly with sources outside the working tree.
	dir := t.TempDir()
	unit := filepath.Join(dir, "vet.cfg")
	depCfg := `{"ImportPath":"example.com/dep","VetxOnly":true,"GoFiles":["` + filepath.Join(dir, "dep.go") + `"]}`
	if err := os.WriteFile(unit, []byte(depCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	got, arguments := unitAnalyzers([]string{"gohawk", "-consumer=true", "-other=true", unit}, selected)
	names := map[string]bool{}
	for _, analyzer := range got {
		names[analyzer.Name] = true
	}
	if !names["facts"] || names["consumer"] || names["other"] {
		t.Fatalf("facts-only unit ran %v, want the producer closure without the consumer", names)
	}
	if !names["buildssa"] {
		t.Fatalf("facts-only unit dropped the producer's prerequisite: %v", names)
	}
	for _, argument := range arguments {
		if argument == "-consumer=true" || argument == "-other=true" {
			t.Fatalf("facts-only unit kept a dropped analyzer's flag: %v", arguments)
		}
	}

	if err := os.WriteFile(unit, []byte(`{"ImportPath":"example.com/root","VetxOnly":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := unitAnalyzers([]string{"gohawk", unit}, selected); len(got) != 2 {
		t.Fatalf("root unit ran %v, want every selected analyzer", got)
	}

	// A first-party package analyzed VetxOnly for its siblings: its sources
	// are under the working tree, so it must not be filtered.
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	firstParty := `{"ImportPath":"example.com/local","VetxOnly":true,"GoFiles":["` + filepath.Join(working, "local.go") + `"]}`
	if err := os.WriteFile(unit, []byte(firstParty), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := unitAnalyzers([]string{"gohawk", unit}, selected); len(got) != 2 {
		t.Fatalf("first-party VetxOnly unit ran %v, want every selected analyzer", got)
	}
	if got, _ := unitAnalyzers([]string{"gohawk", "-flags"}, selected); len(got) != 2 {
		t.Fatalf("flag handshake ran %v, want every selected analyzer", got)
	}
}
