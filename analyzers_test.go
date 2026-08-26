package gohawk

import (
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

var analyzerNames = []string{
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
	"determinism",
	"cancellationownership",
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
	if !slices.Equal(names, analyzerNames) {
		t.Fatalf("registered analyzers = %v, want %v", names, analyzerNames)
	}
}

func TestAnalyzers(t *testing.T) {
	tests := []struct {
		name     string
		packages []string
	}{
		{name: "apishape", packages: []string{"apishape"}},
		{name: "contextpolicy", packages: []string{"contextpolicy"}},
		{name: "globalstate", packages: []string{"globalstate"}},
		{name: "wirepolicy", packages: []string{"wirepolicy"}},
		{name: "testpolicy", packages: []string{"testpolicy"}},
		{name: "blockingtest", packages: []string{"blockingtest"}},
		{name: "goroutineownership", packages: []string{"goroutineownership"}},
		{name: "errorownership", packages: []string{"errorownership"}},
		{name: "channelpolicy", packages: []string{"channelpolicy"}},
		{name: "processownership", packages: []string{"processownership"}},
		{name: "closedomain", packages: []string{"enumfield", "enumfieldsource", "enumfieldconsumer"}},
		{name: "taintpolicy", packages: []string{"taintpolicy"}},
		{name: "lockorder", packages: []string{"lockorder"}},
		{name: "resourcelifetime", packages: []string{"resourcelifetime"}},
		{name: "determinism", packages: []string{"determinism"}},
		{name: "cancellationownership", packages: []string{"cancellationownership"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			analysistest.Run(t, analysistest.TestData(), analyzerNamed(t, test.name), test.packages...)
		})
	}
}
