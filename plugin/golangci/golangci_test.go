package golangci

import (
	"slices"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
)

func TestPluginRegistration(t *testing.T) {
	constructor, err := register.GetPlugin("gohawk")
	if err != nil {
		t.Fatalf("get registered plugin: %v", err)
	}

	linter, err := constructor(nil)
	if err != nil {
		t.Fatalf("construct plugin: %v", err)
	}
	if got := linter.GetLoadMode(); got != register.LoadModeTypesInfo {
		t.Fatalf("load mode = %q, want %q", got, register.LoadModeTypesInfo)
	}

	got, err := linter.BuildAnalyzers()
	if err != nil {
		t.Fatalf("build analyzers: %v", err)
	}
	want := analyzers.DefaultAnalyzers()
	if !slices.Equal(analyzerNames(got), analyzerNames(want)) {
		t.Fatalf("analyzers = %v, want %v", analyzerNames(got), analyzerNames(want))
	}
}

func TestPluginRejectsSettings(t *testing.T) {
	if _, err := New(map[string]any{"unknown": true}); err == nil {
		t.Fatal("New accepted an unknown setting")
	}
}

func TestPluginAnalyzerSelection(t *testing.T) {
	linter, err := New(map[string]any{
		"enable":  []string{"globalstate"},
		"disable": []string{"contextpolicy"},
	})
	if err != nil {
		t.Fatalf("construct configured plugin: %v", err)
	}

	got, err := linter.BuildAnalyzers()
	if err != nil {
		t.Fatalf("build analyzers: %v", err)
	}
	names := analyzerNames(got)
	if slices.Contains(names, "contextpolicy") {
		t.Fatalf("disabled analyzer is present: %v", names)
	}
	if !slices.Contains(names, "globalstate") {
		t.Fatalf("enabled analyzer is absent: %v", names)
	}
}

func TestPluginRejectsUnknownAnalyzer(t *testing.T) {
	if _, err := New(map[string]any{"enable": []string{"not-an-analyzer"}}); err == nil {
		t.Fatal("New accepted an unknown analyzer")
	}
}

func analyzerNames(values []*analysis.Analyzer) []string {
	result := make([]string, len(values))
	for index, analyzer := range values {
		result[index] = analyzer.Name
	}
	return result
}
