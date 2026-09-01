package analyzers

import (
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/catalog"
	"golang.org/x/tools/go/analysis"
)

func expectedAnalyzerNames() []string {
	return []string{
		"apishape",
		"contextpolicy",
		"globalstate",
		"wirepolicy",
		"testpolicy",
		"goroutineownership",
		"errorownership",
		"channelpolicy",
		"processownership",
		"closedomain",
		"taintpolicy",
		"lockorder",
		"resourcelifetime",
		"deferinloop",
		"exitpolicy",
		"determinism",
		"concurrentcapture",
		"evalorder",
		"oncepolicy",
		"syncmapatomicity",
		"cancellationownership",
	}
}

func TestCatalogDeclaration(t *testing.T) {
	if _, err := newCatalog(); err != nil {
		t.Fatalf("invalid analyzer catalog: %v", err)
	}
}

func TestUnknownDiagnosticReturnsError(t *testing.T) {
	analyzer := withSuppressions(&analysis.Analyzer{
		Name: "example",
		Run: func(pass *analysis.Pass) (any, error) {
			pass.Report(analysis.Diagnostic{Category: "example/unknown"})
			return nil, nil
		},
	}, []catalog.CheckInfo{{ID: "example/known"}})

	_, err := analyzer.Run(&analysis.Pass{Report: func(analysis.Diagnostic) {}})
	if err == nil || !strings.Contains(err.Error(), `analyzer "example" reported unknown check "example/unknown"`) {
		t.Fatalf("Run() error = %v, want unknown-check error", err)
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
			name:      "contracts",
			doc:       "API and data contracts",
			docPath:   "api-and-data-contracts",
			analyzers: []string{"apishape", "contextpolicy", "closedomain", "wirepolicy"},
		},
		{
			name:    "ownership",
			doc:     "ownership and lifecycle",
			docPath: "ownership-and-lifecycle",
			analyzers: []string{
				"cancellationownership",
				"channelpolicy",
				"deferinloop",
				"exitpolicy",
				"goroutineownership",
				"processownership",
				"resourcelifetime",
			},
		},
		{
			name:    "reliability",
			doc:     "reliability and safety",
			docPath: "reliability-and-safety",
			analyzers: []string{
				"concurrentcapture",
				"determinism",
				"errorownership",
				"evalorder",
				"globalstate",
				"lockorder",
				"oncepolicy",
				"syncmapatomicity",
				"taintpolicy",
			},
		},
		{name: "testing", doc: "testing", docPath: "testing", analyzers: []string{"testpolicy"}},
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
	optIn := map[string]bool{
		"apishape": true, "closedomain": true,
		"determinism": true, "globalstate": true, "taintpolicy": true,
		"testpolicy": true, "wirepolicy": true,
	}
	seenChecks := make(map[AnalyzerCheck]string)
	optInChecks := map[AnalyzerCheck]bool{
		"channelpolicy/capacity-rationale": true,
		"contextpolicy/test-context":       true,
		"errorownership/log-and-return":    true,
		"goroutineownership/detached":      true,
	}
	for _, name := range expectedAnalyzerNames() {
		info, ok := metadata[name]
		if !ok {
			t.Errorf("metadata missing analyzer %q", name)
		} else if info.OptIn != optIn[name] {
			t.Errorf("analyzer %q opt-in = %t, want %t", name, info.OptIn, optIn[name])
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
			if check.OptIn != optInChecks[check.ID] {
				t.Errorf("check %q opt-in = %t, want %t", check.ID, check.OptIn, optInChecks[check.ID])
			}
			if owner, exists := seenChecks[check.ID]; exists {
				t.Errorf("check %q belongs to both %q and %q", check.ID, owner, name)
			}
			seenChecks[check.ID] = name
		}
	}
}

func TestDefaultAnalyzers(t *testing.T) {
	want := []string{
		"contextpolicy", "goroutineownership", "errorownership",
		"channelpolicy", "processownership", "lockorder", "resourcelifetime",
		"deferinloop", "exitpolicy", "concurrentcapture",
		"evalorder", "oncepolicy", "syncmapatomicity", "cancellationownership",
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
