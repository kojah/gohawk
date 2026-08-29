package analysisutil

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

func TestSourceSSAFunctionsRejectsUnexpectedPrerequisiteResult(t *testing.T) {
	pass := &analysis.Pass{
		ResultOf: map[*analysis.Analyzer]any{
			buildssa.Analyzer: struct{}{},
		},
	}

	functions, err := SourceSSAFunctions(pass)
	if err == nil {
		t.Fatal("SourceSSAFunctions() error = nil, want unexpected buildssa result error")
	}
	if functions != nil {
		t.Fatalf("SourceSSAFunctions() functions = %v, want nil", functions)
	}
}
