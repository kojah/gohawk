package analyzers

import (
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/catalog"
	"golang.org/x/tools/go/analysis"
)

func expectedAnalyzerNames() []string {
	return []string{
		"goroutineownership",
		"producerlifecycle",
		"channelsafety",
		"processownership",
		"lockorder",
		"resourcelifetime",
		"deferinloop",
		"concurrentcapture",
		"cancellationownership",
	}
}

func TestCatalogDeclaration(t *testing.T) {
	if _, err := newCatalog(); err != nil {
		t.Fatalf("invalid analyzer catalog: %v", err)
	}
}

func TestUnknownDiagnosticReturnsError(t *testing.T) {
	analyzer := withSuppressions(catalog.AnalyzerSpec{
		Analyzer: &analysis.Analyzer{
			Name: "example",
			Run: func(pass *analysis.Pass) (any, error) {
				pass.Report(analysis.Diagnostic{Category: "example/unknown"})
				return nil, nil
			},
		},
		Checks: []catalog.CheckInfo{{ID: "example/known", Kind: catalog.KindDefect, Tier: catalog.TierCore}},
	})

	_, err := analyzer.Run(&analysis.Pass{Report: func(analysis.Diagnostic) {}})
	if err == nil || !strings.Contains(err.Error(), `analyzer "example" reported unknown check "example/unknown"`) {
		t.Fatalf("Run() error = %v, want unknown-check error", err)
	}
}

func analyzerTier(name string, extended, experimental map[string]bool) CheckTier {
	switch {
	case experimental[name]:
		return CheckTierExperimental
	case extended[name]:
		return CheckTierExtended
	}
	return CheckTierCore
}

func TestDefaultSuppressionsExcludeSelectedTierChecks(t *testing.T) {
	analyzer := withDefaultSuppressions(catalog.AnalyzerSpec{
		Analyzer: &analysis.Analyzer{
			Name: "example",
			Run: func(pass *analysis.Pass) (any, error) {
				pass.Report(analysis.Diagnostic{Category: "example/default"})
				pass.Report(analysis.Diagnostic{Category: "example/optional"})
				return nil, nil
			},
		},
		Checks: []catalog.CheckInfo{
			{ID: "example/default", Kind: catalog.KindDefect, Tier: catalog.TierCore},
			{ID: "example/optional", Kind: catalog.KindHazard, Tier: catalog.TierExtended},
		},
	})
	var categories []string
	_, err := analyzer.Run(&analysis.Pass{Report: func(diagnostic analysis.Diagnostic) {
		categories = append(categories, diagnostic.Category)
	}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !slices.Equal(categories, []string{"example/default"}) {
		t.Fatalf("reported categories = %v, want only default check", categories)
	}
}

func TestAnalyzerGroups(t *testing.T) {
	want := []struct {
		name      string
		doc       string
		docPath   string
		analyzers []string
	}{
		{
			name:    "concurrency",
			doc:     "concurrency and synchronization",
			docPath: "concurrency-and-synchronization",
			analyzers: []string{
				"channelsafety",
				"concurrentcapture",
				"goroutineownership",
				"lockorder",
				"producerlifecycle",
			},
		},
		{
			name:    "resources",
			doc:     "resources and lifecycle",
			docPath: "resources-and-lifecycle",
			analyzers: []string{
				"cancellationownership",
				"deferinloop",
				"processownership",
				"resourcelifetime",
			},
		},
	}
	groups := AnalyzerGroups()
	if len(groups) != len(want) {
		t.Fatalf("group count = %d, want %d", len(groups), len(want))
	}
	seen := make(map[string]bool)
	for index, group := range groups {
		names := make([]string, 0, len(group.Analyzers))
		for _, analyzer := range group.Analyzers {
			if seen[analyzer.Name] {
				t.Fatalf("analyzer %q appears in more than one group", analyzer.Name)
			}
			seen[analyzer.Name] = true
			names = append(names, analyzer.Name)
		}
		if group.Name != want[index].name || group.Doc != want[index].doc || group.DocPath != want[index].docPath ||
			!slices.Equal(names, want[index].analyzers) {
			t.Fatalf(
				"group %d = %q (%q, %q) %v, want %q (%q, %q) %v",
				index,
				group.Name,
				group.Doc,
				group.DocPath,
				names,
				want[index].name,
				want[index].doc,
				want[index].docPath,
				want[index].analyzers,
			)
		}
	}
	if len(seen) != len(expectedAnalyzerNames()) {
		t.Fatalf("grouped analyzer count = %d, want %d", len(seen), len(expectedAnalyzerNames()))
	}
}

func TestAnalyzerMetadata(t *testing.T) {
	metadata := AnalyzerMetadata()
	if len(metadata) != len(expectedAnalyzerNames()) {
		t.Fatalf("metadata count = %d, want %d", len(metadata), len(expectedAnalyzerNames()))
	}
	extended := map[string]bool{}
	experimental := map[string]bool{}
	seenChecks := make(map[AnalyzerCheck]string)
	checkTiers := map[AnalyzerCheck]CheckTier{
		"resourcelifetime/use-after-release":  CheckTierCore,
		"lockorder/contradictory-order":       CheckTierCore,
		"lockorder/lock-and-join":             CheckTierExperimental,
		"lockorder/channel-lock-cycle":        CheckTierExperimental,
		"lockorder/waitgroup-lock-cycle":      CheckTierExperimental,
		"channelsafety/dependency-cycle":      CheckTierExperimental,
		"lockorder/read-lock-write":           CheckTierExperimental,
		"lockorder/mismatched-release":        CheckTierExperimental,
		"producerlifecycle/stopped-loop-send": CheckTierExperimental,
		"producerlifecycle/unclosed-range":    CheckTierExperimental,
	}
	kinds := map[AnalyzerCheck]CheckKind{
		"cancellationownership/release":       CheckKindDefect,
		"channelsafety/send-after-close":      CheckKindDefect,
		"channelsafety/dependency-cycle":      CheckKindDefect,
		"deferinloop/cleanup-lifetime":        CheckKindHazard,
		"goroutineownership/unjoined":         CheckKindHazard,
		"producerlifecycle/abandoned-send":    CheckKindHazard,
		"producerlifecycle/stopped-loop-send": CheckKindHazard,
		"producerlifecycle/unclosed-range":    CheckKindHazard,
		"processownership/missing-wait":       CheckKindDefect,
		"resourcelifetime/missing-release":    CheckKindDefect,
		"resourcelifetime/use-after-release":  CheckKindHazard,
		"concurrentcapture/shared-capture":    CheckKindHazard,
		"lockorder/missing-release":           CheckKindDefect,
		"lockorder/recursive-acquire":         CheckKindDefect,
		"lockorder/contradictory-order":       CheckKindHazard,
		"lockorder/lock-and-join":             CheckKindDefect,
		"lockorder/channel-lock-cycle":        CheckKindDefect,
		"lockorder/waitgroup-lock-cycle":      CheckKindDefect,
		"lockorder/read-lock-write":           CheckKindHazard,
		"lockorder/mismatched-release":        CheckKindDefect,
	}
	for _, name := range expectedAnalyzerNames() {
		info, ok := metadata[name]
		if !ok {
			t.Errorf("metadata missing analyzer %q", name)
		} else if want := analyzerTier(name, extended, experimental); info.Tier() != want {
			t.Errorf("analyzer %q tier = %q, want %q", name, info.Tier(), want)
		}
		if len(info.Checks) == 0 {
			t.Errorf("analyzer %q has no checks", name)
		}
		for _, check := range info.Checks {
			if check.ID == "" || !strings.HasPrefix(string(check.ID), name+"/") {
				t.Errorf("analyzer %q has invalid check identity %q", name, check.ID)
			}
			if strings.TrimSpace(check.Doc) == "" {
				t.Errorf("check %q has no description", check.ID)
			}
			wantTier, named := checkTiers[check.ID]
			if !named {
				wantTier = analyzerTier(name, extended, experimental)
			}
			if check.Tier != wantTier {
				t.Errorf("check %q tier = %q, want %q", check.ID, check.Tier, wantTier)
			}
			if check.Kind != kinds[check.ID] {
				t.Errorf("check %q kind = %q, want %q", check.ID, check.Kind, kinds[check.ID])
			}
			if owner, exists := seenChecks[check.ID]; exists {
				t.Errorf("check %q belongs to both %q and %q", check.ID, owner, name)
			}
			seenChecks[check.ID] = name
		}
	}
	if len(seenChecks) != len(kinds) {
		t.Fatalf("classified check count = %d, want %d", len(seenChecks), len(kinds))
	}
}

func TestDefaultAnalyzers(t *testing.T) {
	want := []string{
		"goroutineownership", "producerlifecycle",
		"channelsafety",
		"processownership", "lockorder", "resourcelifetime",
		"deferinloop", "concurrentcapture",
		"cancellationownership",
	}
	var names []string
	for _, analyzer := range DefaultAnalyzers() {
		names = append(names, analyzer.Name)
	}
	if !slices.Equal(names, want) {
		t.Fatalf("default analyzers = %v, want %v", names, want)
	}
}

func TestAnalyzerRegistry(t *testing.T) {
	registered := Analyzers()
	names := make([]string, 0, len(registered))
	for _, analyzer := range registered {
		names = append(names, analyzer.Name)
	}
	want := expectedAnalyzerNames()
	if !slices.Equal(names, want) {
		t.Fatalf("registered analyzers = %v, want %v", names, want)
	}
}

func TestTestFileDiagnosticsSkippedByDefault(t *testing.T) {
	fset := token.NewFileSet()
	testFile := fset.AddFile("example_test.go", -1, 100)
	testFile.SetLinesForContent([]byte("package example\n"))
	productionFile := fset.AddFile("example.go", -1, 100)
	productionFile.SetLinesForContent([]byte("package example\n"))
	run := func(pass *analysis.Pass) (any, error) {
		pass.Report(analysis.Diagnostic{Category: "example/check", Pos: testFile.Pos(0)})
		pass.Report(analysis.Diagnostic{Category: "example/check", Pos: productionFile.Pos(0)})
		return nil, nil
	}
	checks := []catalog.CheckInfo{{ID: "example/check", Kind: catalog.KindDefect, Tier: catalog.TierCore}}
	analyzer := withSuppressions(catalog.AnalyzerSpec{Analyzer: &analysis.Analyzer{Name: "example", Run: run}, Checks: checks})
	reported := 0
	if _, err := analyzer.Run(&analysis.Pass{Fset: fset, Report: func(analysis.Diagnostic) { reported++ }}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reported != 1 {
		t.Fatalf("reported %d diagnostics, want 1", reported)
	}
}
