package gohawk

import (
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
		"determinism",
		"concurrentcapture",
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
		{name: "ownership", doc: "ownership and lifecycle", analyzers: []string{"cancellationownership", "channelpolicy", "deferinloop", "goroutineownership", "processownership", "resourcelifetime"}},
		{name: "reliability", doc: "reliability and safety", analyzers: []string{"concurrentcapture", "determinism", "errorownership", "globalstate", "lockorder", "taintpolicy"}},
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
	for _, name := range expectedAnalyzerNames() {
		if _, ok := metadata[name]; !ok {
			t.Errorf("metadata missing analyzer %q", name)
		}
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
		{name: "apishape", packages: []string{"apishape", "docapishape"}},
		{name: "contextpolicy", packages: []string{"contextpolicy", "contextpolicyprod", "doccontextpolicy"}},
		{name: "globalstate", packages: []string{"globalstate", "docglobalstate"}},
		{name: "wirepolicy", packages: []string{"wirepolicy", "docwirepolicy"}},
		{name: "testpolicy", packages: []string{"testpolicy", "doctestpolicy"}},
		{name: "blockingtest", packages: []string{"blockingtest", "docblockingtest"}},
		{name: "goroutineownership", packages: []string{"goroutineownership", "docgoroutineownership"}},
		{name: "errorownership", packages: []string{"errorownership", "docerrorownership"}},
		{name: "channelpolicy", packages: []string{"channelpolicy", "docchannelpolicy"}},
		{name: "processownership", packages: []string{"processownership", "docprocessownership"}},
		{name: "closedomain", packages: []string{"enumfield", "enumfieldsource", "enumfieldconsumer", "docclosedomain"}},
		{name: "taintpolicy", packages: []string{"taintpolicy", "doctaintpolicy"}},
		{name: "lockorder", packages: []string{"lockorder", "doclockorder"}},
		{name: "resourcelifetime", packages: []string{"resourcelifetime", "docresourcelifetime"}},
		{name: "deferinloop", packages: []string{"deferinloop", "docdeferinloop"}},
		{name: "determinism", packages: []string{"determinism", "docdeterminism"}},
		{name: "concurrentcapture", packages: []string{"concurrentcapture", "docconcurrentcapture"}},
		{name: "cancellationownership", packages: []string{"cancellationownership", "doccancellationownership"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			analysistest.Run(t, analysistest.TestData(), requireDiagnosticRanges(t, analyzerNamed(t, test.name)), test.packages...)
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
		{name: "wirepolicy", pattern: "wirepolicyfix"},
		{name: "testpolicy", pattern: "testpolicyfix"},
		{name: "cancellationownership", pattern: "cancellationownershipfix"},
	}
	tested := make(map[string]bool, len(tests))
	for _, test := range tests {
		tested[test.name] = true
		if !metadata[test.name].SuggestedFix {
			t.Errorf("analyzer %q has a suggested-fix test but is not marked as offering one", test.name)
		}
		t.Run(test.name, func(t *testing.T) {
			analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), analyzerNamed(t, test.name), test.pattern)
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
	analysistest.Run(t, analysistest.TestData(), analyzer, "goroutineownershipstrict")
}

func TestGoroutineOwnershipLifecycleMode(t *testing.T) {
	analyzer := configurableAnalyzer(t, "goroutineownership")
	if err := analyzer.Flags.Set("mode", "lifecycle"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "goroutineownershiplifecycle")
}

func TestAnalyzerConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		flags   map[string]string
	}{
		{name: "apishape", pattern: "apishapeconfig", flags: map[string]string{
			"max-parameters": "5", "max-adjacent-same-type": "5",
		}},
		{name: "channelpolicy", pattern: "channelpolicyconfig", flags: map[string]string{
			"max-unexplained-capacity": "10",
		}},
		{name: "contextpolicy", pattern: "contextpolicyconfig", flags: map[string]string{
			"prefer-test-context": "false",
		}},
		{name: "globalstate", pattern: "globalstateconfig", flags: map[string]string{
			"allow-names": "cache", "allow-types": "globalstateconfig.Registry",
		}},
		{name: "taintpolicy", pattern: "taintpolicyconfig", flags: map[string]string{
			"sinks": "process", "sanitizers": "taintpolicyconfig.scrub",
		}},
		{name: "resourcelifetime", pattern: "resourcelifetimeconfig", flags: map[string]string{
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
			analysistest.Run(t, analysistest.TestData(), analyzer, test.pattern)
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
