package testlifecycle

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(func(*ssa.Go) bool { return false }), "testlifecycle")
}

func TestAnalyzerUsesInjectedOwnershipProof(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(func(*ssa.Go) bool { return true }), "testlifecycle/owned")
}

func TestAnalyzerRequiresOwnershipProof(t *testing.T) {
	_, err := Analyzer(nil).Run(nil)
	if err == nil || !strings.Contains(err.Error(), "requires a goroutine ownership proof") {
		t.Fatalf("Run() error = %v, want missing ownership proof", err)
	}
}
