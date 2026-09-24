package goroutineownership

import "testing"

func TestSummaryJoinReasonCodes(t *testing.T) {
	want := map[summaryJoinReason]string{
		summaryJoinNone: "",
		summaryJoinConcurrencyJoinBudgetExhausted: "concurrency-join-budget-exhausted",
		summaryJoinConcurrencyJoinNotApplicable:   "concurrency-join-not-applicable",
		summaryJoinConcurrencyJoinNotSynchronous:  "concurrency-join-not-synchronous",
		summaryJoinConcurrencySummaryJoin:         "concurrency-summary-join",
		summaryJoinConcurrencySummaryNoExactJoin:  "concurrency-summary-no-exact-join",
	}
	if len(want) != int(summaryJoinReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range summaryJoinReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []summaryJoinReason{summaryJoinReasonCount, 255} {
		if reason.String() != "invalid-goroutineownership-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
