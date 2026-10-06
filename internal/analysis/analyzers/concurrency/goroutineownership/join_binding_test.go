package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestJoinBindingStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"exactHelper":         GoroutineLifecycleHonored,
		"mixedHelper":         GoroutineUnknown,
		"replacedHelperField": GoroutineUnknown,
		"exactWait":           GoroutineLifecycleHonored,
		"mixedWait":           GoroutineUnknown,
		"unrelatedHelper":     GoroutineLifecycleViolated,
		"mixedOwner":          GoroutineUnknown,
	}
	assertSpawnProofs(t, want, "joinbindings")
}

func TestJoinReceiverTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "joinbindings", "ownerparticipation")
	want := map[string][3]string{
		"joinbindings.mixedWait":           {"possible-join-receiver", "unknown", "opaque-use"},
		"joinbindings.mixedOwner":          {"owner-lifecycle-participation", "unknown", "opaque-use"},
		"joinbindings.exactWait":           {"direct-join", "accepted", "join"},
		"ownerparticipation.directClose":   {"owner-lifecycle-participation", "unknown", "opaque-use"},
		"ownerparticipation.deferredClose": {"owner-lifecycle-participation", "unknown", "opaque-use"},
		"ownerparticipation.helperClose":   {"helper-use", "unknown", "opaque-use"},
	}
	assertClassifierLabels(t, path, want)
}
