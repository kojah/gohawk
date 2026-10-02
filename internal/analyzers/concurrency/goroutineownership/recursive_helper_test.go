package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestRecursiveHelperStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"recursiveObserved":   GoroutineUnknown,
		"mutualObserved":      GoroutineUnknown,
		"exactAfterRecursive": GoroutineLifecycleHonored,
		"unrelatedRecursive":  GoroutineLifecycleViolated,
	}, "recursivehelpers")
}

func TestRecursiveHelperTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "recursivehelpers")
	assertClassifierLabels(t, path, map[string][3]string{
		"recursivehelpers.recursiveObserved":   {"helper-use", "unknown", "opaque-use"},
		"recursivehelpers.mutualObserved":      {"helper-use", "unknown", "opaque-use"},
		"recursivehelpers.exactAfterRecursive": {"helper-use", "accepted", "join"},
	})
}
