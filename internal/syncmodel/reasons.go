package syncmodel

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
)

// Reason classifies evidence owned by the synchronization graph.
type Reason uint8

const (
	ReasonNone Reason = iota
	ReasonAlternateUnlock
	ReasonAlternativeLimit
	ReasonCancelIdentityUnknown
	ReasonCancellationObservations
	ReasonEventUnavailable
	ReasonFreshResource
	ReasonFreshnessUnknown
	ReasonHeldMutex
	ReasonHeldMutexUnknown
	ReasonIdentityUnknown
	ReasonInvalidEvent
	ReasonInvalidSpawnPrefix
	ReasonNestedAlternatives
	ReasonNoChannelSignal
	ReasonOrderUnproven
	ReasonProgramOrder
	ReasonQueryUnavailable
	ReasonScopeAliasUnknown
	ReasonScopeDependencyPresent
	ReasonScopeIncomplete
	ReasonSignalBeforeAcquire
	reasonCount
)

var reasonCodes = [...]string{
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
}

// String renders the stable external trace code.
func (reason Reason) String() string {
	if int(reason) >= len(reasonCodes) {
		return "invalid-sync-reason"
	}
	return reasonCodes[reason]
}

// Failure preserves either a graph rejection or its upstream summary cause.
// Its zero value means no failure. Construction keeps the two domains exclusive.
type Failure struct {
	graph   Reason
	summary concurrencyfacts.Reason
}

func graphFailure(reason Reason) Failure                    { return Failure{graph: reason} }
func summaryFailure(reason concurrencyfacts.Reason) Failure { return Failure{summary: reason} }

// Empty reports whether neither domain rejected the evidence.
func (failure Failure) Empty() bool { return failure == (Failure{}) }

// String renders the originating domain's code only at an output boundary.
func (failure Failure) String() string {
	if failure.graph != ReasonNone {
		return failure.graph.String()
	}
	return failure.summary.String()
}

// Proof carries a query outcome and its domain-owned explanation. An unavailable
// graph retains its typed upstream failure instead of inventing query evidence.
type Proof struct {
	State   ssaflow.EvidenceState
	Reason  Reason
	Failure Failure
}

// Proven reports positive evidence, not merely absence of a rejection.
func (proof Proof) Proven() bool { return proof.State == ssaflow.EvidenceProven }

// Known distinguishes proven/disproven evidence from an unknown query.
func (proof Proof) Known() bool { return proof.State != ssaflow.EvidenceUnknown }
