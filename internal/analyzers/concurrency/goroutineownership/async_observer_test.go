package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAsyncObserverStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"directAsyncWait":    GoroutineUnknown,
		"helperAsyncWait":    GoroutineUnknown,
		"directWait":         GoroutineLifecycleHonored,
		"deferredWait":       GoroutineLifecycleHonored,
		"helperDeferredWait": GoroutineLifecycleHonored,
		"launchedHelper":     GoroutineUnknown,
		"unrelatedObserver":  GoroutineLifecycleViolated,
	}, "asyncobservers")
}

func TestAsyncObserverTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "asyncobservers")
	want := map[string][3]string{
		"asyncobservers.directAsyncWait": {"launched-helper", "unknown", "opaque-use"},
		"asyncobservers.helperAsyncWait": {"helper-use", "unknown", "opaque-use"},
		"asyncobservers.launchedHelper":  {"launched-helper", "unknown", "opaque-use"},
		"asyncobservers.directWait":      {"direct-join", "accepted", "join"},
		"asyncobservers.deferredWait":    {"direct-join", "accepted", "join"},
	}
	assertClassifierLabels(t, path, want)
}
