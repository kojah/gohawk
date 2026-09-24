package check

import "testing"

func TestReportingReasonCodes(t *testing.T) {
	want := map[ReportingReason]string{
		ReportingNone: "", ReportingCandidate: "diagnostic-candidate",
		ReportingDisabled: "check-disabled", ReportingEmitted: "diagnostic-reported",
	}
	if len(want) != int(reportingReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range reportingReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []ReportingReason{reportingReasonCount, 255} {
		if reason.String() != "invalid-reporting-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
