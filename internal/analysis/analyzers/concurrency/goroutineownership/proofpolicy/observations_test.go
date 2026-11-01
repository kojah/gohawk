package proofpolicy_test

import (
	"reflect"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/analyzers/concurrency/goroutineownership/proofpolicy"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

type callerQueries struct {
	calls        []string
	observations map[string]proofpolicy.LifetimeObservation
	cutoff       proofpolicy.Proof
}

func (q *callerQueries) observe(name string) proofpolicy.LifetimeObservation {
	q.calls = append(q.calls, name)
	return q.observations[name]
}
func (q *callerQueries) Stop() proofpolicy.LifetimeObservation     { return q.observe("stop") }
func (q *callerQueries) Context() proofpolicy.LifetimeObservation  { return q.observe("context") }
func (q *callerQueries) Canceled() proofpolicy.LifetimeObservation { return q.observe("canceled") }
func (q *callerQueries) Receiver() proofpolicy.LifetimeObservation { return q.observe("receiver") }
func (q *callerQueries) Group() proofpolicy.LifetimeObservation    { return q.observe("group") }
func (q *callerQueries) Cutoff() proofpolicy.Proof {
	q.calls = append(q.calls, "cutoff")
	return q.cutoff
}

func TestCallerLifetimeStopsAtFirstWitnessOrCutoff(t *testing.T) {
	stages := []string{"stop", "context", "canceled", "receiver", "group"}
	reasons := []proofpolicy.Reason{
		proofpolicy.StopLifecycle, proofpolicy.ContextLifecycle, proofpolicy.LocallyCanceledContext, proofpolicy.ReceiverContext,
		proofpolicy.WaitGroupBound,
	}
	for index, stage := range stages {
		for _, interrupted := range []bool{false, true} {
			t.Run(stage, func(t *testing.T) {
				host := &callerQueries{observations: map[string]proofpolicy.LifetimeObservation{}, cutoff: proofpolicy.Proof{
					Outcome: proofpolicy.Unknown, Reason: proofpolicy.ReceiveBudgetExhausted, Witness: &ssa.Return{},
				}}
				// Later positive witnesses overlap with this one. They must not
				// run or replace either its reason or its interrupted payload.
				for _, later := range stages[index:] {
					host.observations[later] = proofpolicy.LifetimeObservation{Found: true}
				}
				host.observations[stage] = proofpolicy.LifetimeObservation{Found: true, Interrupted: interrupted}
				want := proofpolicy.Proof{Outcome: proofpolicy.Unknown, Reason: reasons[index]}
				calls := append([]string(nil), stages[:index+1]...)
				if interrupted {
					want = host.cutoff
					calls = append(calls, "cutoff")
				}
				got, decided := proofpolicy.CallerLifetime(host)
				if !decided || got != want || !reflect.DeepEqual(host.calls, calls) {
					t.Fatalf("proof=%+v decided=%t calls=%v; want %+v, %v", got, decided, host.calls, want, calls)
				}
			})
		}
	}
}

func TestCallerLifetimeAbsentWitnessesRemainUndecided(t *testing.T) {
	host := &callerQueries{}
	got, decided := proofpolicy.CallerLifetime(host)
	if decided || got != (proofpolicy.Proof{}) || !reflect.DeepEqual(host.calls, []string{"stop", "context", "canceled", "receiver", "group"}) {
		t.Fatalf("proof=%+v decided=%t calls=%v", got, decided, host.calls)
	}
}

type edgeQueries struct {
	calls     []string
	selected  bool
	drain     ssapath.ObligationAction
	possible  bool
	ownership bool
}

func (q *edgeQueries) Selected() bool { q.calls = append(q.calls, "selected"); return q.selected }
func (q *edgeQueries) Drain() (ssapath.ObligationAction, bool) {
	q.calls = append(q.calls, "drain")
	return q.drain, q.drain != ssapath.ObligationNone
}
func (q *edgeQueries) Possible() bool  { q.calls = append(q.calls, "possible"); return q.possible }
func (q *edgeQueries) Ownership() bool { q.calls = append(q.calls, "ownership"); return q.ownership }

func TestEdgeQueriesPreserveExactAndOpaquePrecedence(t *testing.T) {
	for _, test := range []struct {
		name    string
		queries edgeQueries
		want    ssapath.ObligationAction
		calls   []string
	}{
		{
			"selected",
			edgeQueries{selected: true, drain: ssapath.ObligationUnknown, possible: true, ownership: true},
			ssapath.ObligationExact,
			[]string{"selected"},
		},
		{
			"exact drain",
			edgeQueries{drain: ssapath.ObligationExact, possible: true, ownership: true},
			ssapath.ObligationExact,
			[]string{"selected", "drain"},
		},
		{
			"opaque drain",
			edgeQueries{drain: ssapath.ObligationUnknown, possible: true, ownership: true},
			ssapath.ObligationUnknown,
			[]string{"selected", "drain"},
		},
		{
			"possible receive",
			edgeQueries{possible: true, ownership: true},
			ssapath.ObligationUnknown,
			[]string{"selected", "drain", "possible"},
		},
		{
			"owner",
			edgeQueries{ownership: true},
			ssapath.ObligationUnknown,
			[]string{"selected", "drain", "possible", "ownership"},
		},
		{"absent", edgeQueries{}, ssapath.ObligationNone, []string{"selected", "drain", "possible", "ownership"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := proofpolicy.Edge(&test.queries)
			if got != test.want || !reflect.DeepEqual(test.queries.calls, test.calls) {
				t.Fatalf("action=%v calls=%v, want %v/%v", got, test.queries.calls, test.want, test.calls)
			}
		})
	}
}
