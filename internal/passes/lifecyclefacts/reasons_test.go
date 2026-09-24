package lifecyclefacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

func TestReasonCodes(t *testing.T) {
	want := map[Reason]string{
		reasonNone:                              "",
		reasonLifecycleSummary:                  "lifecycle-summary",
		reasonLifecycleSummaryProjectedArgument: "lifecycle-summary-projected-argument",
		reasonLifecycleSummaryCapturedArgument:  "lifecycle-summary-captured-argument",
		reasonReceiverStoreTransfer:             "receiver-store-transfer",
		reasonReceiverDoesNotEscape:             "receiver-does-not-escape",
		reasonOwnedResultContract:               "owned-result-contract",
		reasonOwnedResultUnreleasable:           "owned-result-unreleasable",
		reasonStoredByCallee:                    "stored-by-callee",
		reasonConditionalSummary:                "conditional-lifecycle-summary",
		reasonRetentionBudget:                   "retention-budget-exhausted",
		reasonSummarizingFunction:               "summarizing-function",
		reasonFunctionSummarized:                "function-summarized",
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
		if reason.String() != "invalid-lifecycle-summary-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

func TestProofKeepsSummaryAndLocalEvidenceSeparate(t *testing.T) {
	local := Proof{Proof: ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}}
	if local.Known() || local.SummaryReason != reasonNone || local.traceReason() != "budget-exhausted" {
		t.Fatalf("local cutoff changed: %+v", local)
	}
	imported := importedProof(reasonLifecycleSummary, "Close")
	if !imported.Proven() || imported.Reason != ssaflow.EvidenceNone || imported.SummaryReason != reasonLifecycleSummary ||
		imported.Method != "Close" || imported.Provenance != ssaflow.EvidenceFromImportedFact || imported.traceReason() != "lifecycle-summary" {
		t.Fatalf("summary proof changed: %+v", imported)
	}
	if (Proof{}).Known() || (Proof{}).traceReason() != "" {
		t.Fatal("zero proof must stay unknown")
	}
}
