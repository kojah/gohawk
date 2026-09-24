package resultfacts

import "testing"

func TestReasonCodes(t *testing.T) {
	want := map[Reason]string{
		ReasonNone: "", ReasonSummaryUnavailable: "result-summary-unavailable",
		ReasonBodyUnavailable: "result-body-unavailable", ReasonCountLimit: "result-count-limit",
		ReasonBudgetExhausted: "result-budget-exhausted", ReasonNoNormalReturnWitness: "result-no-normal-return-witness",
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
		if reason.String() != "invalid-result-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
