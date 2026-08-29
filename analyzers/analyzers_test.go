package analyzers

import (
	"path/filepath"
	"slices"
	"testing"

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

func TestAnalyzerGroups(t *testing.T) {
	want := []struct {
		name      string
		doc       string
		analyzers []string
	}{
		{name: "contracts", doc: "API and data contracts", analyzers: []string{"apishape", "contextpolicy", "closedomain", "wirepolicy"}},
		{name: "ownership", doc: "ownership and lifecycle", analyzers: []string{"cancellationownership", "channelpolicy", "deferinloop", "exitpolicy", "goroutineownership", "processownership", "resourcelifetime"}},
		{name: "reliability", doc: "reliability and safety", analyzers: []string{"concurrentcapture", "determinism", "errorownership", "evalorder", "globalstate", "lockorder", "oncepolicy", "syncmapatomicity", "taintpolicy"}},
		{name: "testing", doc: "testing", analyzers: []string{"blockingtest", "testpolicy"}},
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
		if group.Name != want[index].name || group.Doc != want[index].doc || !slices.Equal(names, want[index].analyzers) {
			t.Fatalf("group %d = %q (%q) %v, want %q (%q) %v", index, group.Name, group.Doc, names, want[index].name, want[index].doc, want[index].analyzers)
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
		"apishape": true, "closedomain": true, "globalstate": true,
		"taintpolicy": true, "testpolicy": true, "wirepolicy": true,
	}
	validTags := map[AnalyzerTag]bool{
		AnalyzerTagCorrectness: true,
		AnalyzerTagReliability: true,
		AnalyzerTagPolicy:      true,
	}
	for _, name := range expectedAnalyzerNames() {
		info, ok := metadata[name]
		if !ok {
			t.Errorf("metadata missing analyzer %q", name)
		} else if info.EnabledByDefault() == optIn[name] {
			t.Errorf("analyzer %q profile = %q, want default = %t", name, info.Profile, !optIn[name])
		}
		if len(info.Tags) == 0 {
			t.Errorf("analyzer %q has no tags", name)
		}
		seenTags := make(map[AnalyzerTag]bool, len(info.Tags))
		for _, tag := range info.Tags {
			if !validTags[tag] {
				t.Errorf("analyzer %q has unknown tag %q", name, tag)
			}
			if seenTags[tag] {
				t.Errorf("analyzer %q repeats tag %q", name, tag)
			}
			seenTags[tag] = true
		}
	}
}

func TestDefaultAnalyzers(t *testing.T) {
	want := []string{
		"contextpolicy", "blockingtest", "goroutineownership", "errorownership",
		"channelpolicy", "processownership", "lockorder", "resourcelifetime",
		"deferinloop", "exitpolicy", "determinism", "concurrentcapture",
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
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if diagnostic.End <= diagnostic.Pos {
				t.Errorf("%s diagnostic %q has no precise range", analyzer.Name, diagnostic.Message)
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
