package analyzers

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzerbase"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func expectedAnalyzerNames() []string {
	return []string{
		"apishape",
		"contextpolicy",
		"globalstate",
		"wirepolicy",
		"testpolicy",
		"blockingtest",
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
	}, []analyzerbase.CheckInfo{{ID: "example/known"}})

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
		{name: "contracts", doc: "API and data contracts", docPath: "api-and-data-contracts", analyzers: []string{"apishape", "contextpolicy", "closedomain", "wirepolicy"}},
		{name: "ownership", doc: "ownership and lifecycle", docPath: "ownership-and-lifecycle", analyzers: []string{"cancellationownership", "channelpolicy", "deferinloop", "exitpolicy", "goroutineownership", "processownership", "resourcelifetime"}},
		{name: "reliability", doc: "reliability and safety", docPath: "reliability-and-safety", analyzers: []string{"concurrentcapture", "determinism", "errorownership", "evalorder", "globalstate", "lockorder", "oncepolicy", "syncmapatomicity", "taintpolicy"}},
		{name: "testing", doc: "testing", docPath: "testing", analyzers: []string{"blockingtest", "testpolicy"}},
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
		if group.Name != want[index].name || group.Doc != want[index].doc || group.DocPath != want[index].docPath || !slices.Equal(names, want[index].analyzers) {
			t.Fatalf("group %d = %q (%q, %q) %v, want %q (%q, %q) %v", index, group.Name, group.Doc, group.DocPath, names, want[index].name, want[index].doc, want[index].docPath, want[index].analyzers)
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
		"apishape": true, "blockingtest": true, "closedomain": true,
		"determinism": true, "globalstate": true, "taintpolicy": true,
		"testpolicy": true, "wirepolicy": true,
	}
	validTags := make(map[AnalyzerTag]bool)
	for _, tag := range TagCatalog() {
		validTags[tag.ID] = true
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
		} else if info.EnabledByDefault() == optIn[name] {
			t.Errorf("analyzer %q profile = %q, want default = %t", name, info.Profile, !optIn[name])
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
			if check.Profile != CheckProfileDefault && check.Profile != CheckProfileOptIn {
				t.Errorf("check %q has invalid profile %q", check.ID, check.Profile)
			}
			wantProfile := CheckProfileDefault
			if optInChecks[check.ID] {
				wantProfile = CheckProfileOptIn
			}
			if check.Profile != wantProfile {
				t.Errorf("check %q profile = %q, want %q", check.ID, check.Profile, wantProfile)
			}
			if owner, exists := seenChecks[check.ID]; exists {
				t.Errorf("check %q belongs to both %q and %q", check.ID, owner, name)
			}
			seenChecks[check.ID] = name
			if len(check.Tags) == 0 {
				t.Errorf("check %q has no tags", check.ID)
			}
			seenCheckTags := make(map[AnalyzerTag]bool, len(check.Tags))
			for _, tag := range check.Tags {
				if !validTags[tag] {
					t.Errorf("check %q has unknown tag %q", check.ID, tag)
				}
				if seenCheckTags[tag] {
					t.Errorf("check %q repeats tag %q", check.ID, tag)
				}
				seenCheckTags[tag] = true
			}
		}
	}
}

func TestTagCatalog(t *testing.T) {
	want := []AnalyzerTag{AnalyzerTagCorrectness, AnalyzerTagReliability, AnalyzerTagPolicy}
	catalog := TagCatalog()
	if len(catalog) != len(want) {
		t.Fatalf("tag count = %d, want %d", len(catalog), len(want))
	}
	seen := make(map[AnalyzerTag]bool, len(catalog))
	for index, tag := range catalog {
		if tag.ID != want[index] {
			t.Errorf("tag %d = %q, want %q", index, tag.ID, want[index])
		}
		if tag.ID == "" || strings.TrimSpace(tag.Description) == "" {
			t.Errorf("tag %d is missing an identity or description: %+v", index, tag)
		}
		if seen[tag.ID] {
			t.Errorf("tag %q is defined more than once", tag.ID)
		}
		seen[tag.ID] = true
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

func analyzerNamed(t *testing.T, name string) *analysis.Analyzer {
	t.Helper()
	for _, analyzer := range Analyzers() {
		if analyzer.Name == name {
			return analyzer
		}
	}
	t.Fatalf("analyzer %q is not registered", name)
	return nil
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

func TestAnalyzers(t *testing.T) {
	tests := []struct {
		name     string
		packages []string
	}{
		{name: "apishape", packages: []string{"apishape"}},
		{name: "contextpolicy", packages: []string{"contextpolicy", "contextpolicy/production"}},
		{name: "globalstate", packages: []string{"globalstate"}},
		{name: "wirepolicy", packages: []string{"wirepolicy"}},
		{name: "testpolicy", packages: []string{"testpolicy"}},
		{name: "blockingtest", packages: []string{"blockingtest"}},
		{name: "goroutineownership", packages: []string{"goroutineownership"}},
		{name: "errorownership", packages: []string{"errorownership"}},
		{name: "channelpolicy", packages: []string{"channelpolicy"}},
		{name: "processownership", packages: []string{"processownership"}},
		{name: "closedomain", packages: []string{"closedomain", "closedomain/cases", "closedomain/source", "closedomain/consumer"}},
		{name: "taintpolicy", packages: []string{"taintpolicy"}},
		{name: "lockorder", packages: []string{"lockorder"}},
		{name: "resourcelifetime", packages: []string{"resourcelifetime"}},
		{name: "deferinloop", packages: []string{"deferinloop"}},
		{name: "exitpolicy", packages: []string{"exitpolicy"}},
		{name: "determinism", packages: []string{"determinism"}},
		{name: "concurrentcapture", packages: []string{"concurrentcapture"}},
		{name: "evalorder", packages: []string{"evalorder"}},
		{name: "oncepolicy", packages: []string{"oncepolicy"}},
		{name: "syncmapatomicity", packages: []string{"syncmapatomicity"}},
		{name: "cancellationownership", packages: []string{"cancellationownership"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			analysistest.Run(t, testData(t), requireDiagnosticRanges(t, analyzerNamed(t, test.name)), test.packages...)
		})
	}
}

func requireDiagnosticRanges(t *testing.T, analyzer *analysis.Analyzer) *analysis.Analyzer {
	t.Helper()
	checks := make(map[string]bool)
	for _, check := range AnalyzerMetadata()[analyzer.Name].Checks {
		checks[string(check.ID)] = true
	}
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if diagnostic.End <= diagnostic.Pos {
				t.Errorf("%s diagnostic %q has no precise range", analyzer.Name, diagnostic.Message)
			}
			if !checks[diagnostic.Category] {
				t.Errorf("%s diagnostic %q has unknown check identity %q", analyzer.Name, diagnostic.Message, diagnostic.Category)
			}
			report(diagnostic)
		}
		defer func() { pass.Report = report }()
		return run(pass)
	}
	return analyzer
}

func TestSuggestedFixes(t *testing.T) {
	metadata := AnalyzerMetadata()
	tests := []struct {
		name    string
		pattern string
	}{
		{name: "wirepolicy", pattern: "wirepolicy/fix"},
		{name: "testpolicy", pattern: "testpolicy/fix"},
		{name: "cancellationownership", pattern: "cancellationownership/fix"},
	}
	tested := make(map[string]bool, len(tests))
	for _, test := range tests {
		tested[test.name] = true
		if !metadata[test.name].SuggestedFix {
			t.Errorf("analyzer %q has a suggested-fix test but is not marked as offering one", test.name)
		}
		t.Run(test.name, func(t *testing.T) {
			analysistest.RunWithSuggestedFixes(t, testData(t), analyzerNamed(t, test.name), test.pattern)
		})
	}
	for name, info := range metadata {
		if info.SuggestedFix && !tested[name] {
			t.Errorf("analyzer %q is marked as offering a suggested fix but has no suggested-fix test", name)
		}
	}
}

func TestGoroutineOwnershipJoinMode(t *testing.T) {
	analyzer := configurableAnalyzer(t, "goroutineownership")
	if err := analyzer.Flags.Set("mode", "join"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	analysistest.Run(t, testData(t), analyzer, "goroutineownership/strict")
}

func TestGoroutineOwnershipLifecycleMode(t *testing.T) {
	analyzer := configurableAnalyzer(t, "goroutineownership")
	if err := analyzer.Flags.Set("mode", "lifecycle"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	analysistest.Run(t, testData(t), analyzer, "goroutineownership/lifecycle")
}

func TestAnalyzerConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		flags   map[string]string
	}{
		{name: "apishape", pattern: "apishape/config", flags: map[string]string{
			"max-parameters": "5", "max-adjacent-same-type": "5",
		}},
		{name: "channelpolicy", pattern: "channelpolicy/config", flags: map[string]string{
			"max-unexplained-capacity": "10",
		}},
		{name: "contextpolicy", pattern: "contextpolicy/config", flags: map[string]string{
			"prefer-test-context": "false",
		}},
		{name: "globalstate", pattern: "globalstate/config", flags: map[string]string{
			"allow-names": "cache", "allow-types": "globalstate/config.Registry",
		}},
		{name: "taintpolicy", pattern: "taintpolicy/config", flags: map[string]string{
			"sinks": "process", "sanitizers": "taintpolicy/config.scrub",
		}},
		{name: "resourcelifetime", pattern: "resourcelifetime/config", flags: map[string]string{
			"contracts": "http,compress", "require-reader-close": "false",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analyzer := configurableAnalyzer(t, test.name)
			for name, value := range test.flags {
				if err := analyzer.Flags.Set(name, value); err != nil {
					t.Fatalf("set %s=%s: %v", name, value, err)
				}
			}
			analysistest.Run(t, testData(t), analyzer, test.pattern)
		})
	}
}

func configurableAnalyzer(t *testing.T, name string) *analysis.Analyzer {
	t.Helper()
	for _, group := range AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			if analyzer.Name == name {
				return analyzer
			}
		}
	}
	t.Fatalf("%s analyzer is not registered", name)
	return nil
}

func testData(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}
