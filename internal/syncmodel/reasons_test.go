package syncmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
)

func TestReasonCodes(t *testing.T) {
	want := map[Reason]string{
		ReasonNone:                     "",
		ReasonAlternateUnlock:          "syncgraph-alternate-unlock",
		ReasonAlternativeLimit:         "syncgraph-alternative-limit",
		ReasonCancelIdentityUnknown:    "syncgraph-cancel-identity-unknown",
		ReasonCancellationObservations: "syncgraph-cancellation-observations",
		ReasonEventUnavailable:         "syncgraph-event-unavailable",
		ReasonFreshResource:            "syncgraph-fresh-resource",
		ReasonFreshnessUnknown:         "syncgraph-freshness-unknown",
		ReasonHeldMutex:                "syncgraph-held-mutex",
		ReasonHeldMutexUnknown:         "syncgraph-held-mutex-unknown",
		ReasonIdentityUnknown:          "syncgraph-identity-unknown",
		ReasonInvalidEvent:             "syncgraph-invalid-event",
		ReasonInvalidSpawnPrefix:       "syncgraph-invalid-spawn-prefix",
		ReasonNestedAlternatives:       "syncgraph-nested-alternatives",
		ReasonNoChannelSignal:          "syncgraph-no-channel-signal",
		ReasonOrderUnproven:            "syncgraph-order-unproven",
		ReasonProgramOrder:             "syncgraph-program-order",
		ReasonQueryUnavailable:         "syncgraph-query-unavailable",
		ReasonScopeAliasUnknown:        "syncgraph-scope-alias-unknown",
		ReasonScopeDependencyPresent:   "syncgraph-scope-dependency-present",
		ReasonScopeIncomplete:          "syncgraph-scope-incomplete",
		ReasonSignalBeforeAcquire:      "syncgraph-signal-before-acquire",
		ReasonConditionsFeasible:       "syncgraph-conditions-feasible",
		ReasonConditionsContradict:     "syncgraph-conditions-contradict",
		ReasonConditionsCorrelated:     "syncgraph-conditions-correlated",
		ReasonConditionsUnknown:        "syncgraph-conditions-unknown",
	}
	if len(want) != int(reasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range reasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []Reason{reasonCount, 255} {
		if reason.String() != "invalid-sync-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

func TestFailureKeepsOriginatingEnum(t *testing.T) {
	summary := concurrencyfacts.Summary{Reason: concurrencyfacts.ReasonBudgetExhausted}
	graph := FromSummary(summary)
	_, failure := Expand(summary)
	if failure != graph.Failure || failure.Empty() ||
		failure.summary != concurrencyfacts.ReasonBudgetExhausted || failure.graph != ReasonNone {
		t.Fatalf("summary cause lost: %+v, %+v", graph.Failure, failure)
	}
	proof := NewQuery(graph).Before(0, 1)
	if proof.Known() || proof.Failure != failure || proof.Reason != ReasonNone {
		t.Fatalf("unavailable query invented evidence: %+v", proof)
	}
	if failure.String() != "protocol-budget-exhausted" {
		t.Fatalf("trace code changed: %s", failure)
	}

	local := graphFailure(ReasonInvalidSpawnPrefix)
	if local.Empty() || local.summary != concurrencyfacts.ReasonNone ||
		local.graph != ReasonInvalidSpawnPrefix || local.String() != "syncgraph-invalid-spawn-prefix" {
		t.Fatalf("graph cause lost: %+v", local)
	}
	if !(Failure{}).Empty() || (Failure{}).String() != "" {
		t.Fatal("zero failure must stay empty")
	}
}
