package lifecyclefacts

import "github.com/kojah/gohawk/internal/ssaflow"

// Reason classifies summary evidence, independently of local SSA evidence.
type Reason uint8

const (
	reasonNone Reason = iota
	reasonLifecycleSummary
	reasonLifecycleSummaryProjectedArgument
	reasonLifecycleSummaryCapturedArgument
	reasonReceiverStoreTransfer
	reasonReceiverDoesNotEscape
	reasonOwnedResultContract
	reasonOwnedResultUnreleasable
	reasonRetainingResultContract
	reasonStoredByCallee
	reasonConditionalSummary
	reasonRetentionBudget
	reasonSummarizingFunction
	reasonFunctionSummarized
	reasonCount
)

var reasonCodes = [...]string{
	reasonNone:                              "",
	reasonLifecycleSummary:                  "lifecycle-summary",
	reasonLifecycleSummaryProjectedArgument: "lifecycle-summary-projected-argument",
	reasonLifecycleSummaryCapturedArgument:  "lifecycle-summary-captured-argument",
	reasonReceiverStoreTransfer:             "receiver-store-transfer",
	reasonReceiverDoesNotEscape:             "receiver-does-not-escape",
	reasonOwnedResultContract:               "owned-result-contract",
	reasonOwnedResultUnreleasable:           "owned-result-unreleasable",
	reasonRetainingResultContract:           "retaining-result-contract",
	reasonStoredByCallee:                    "stored-by-callee",
	reasonConditionalSummary:                "conditional-lifecycle-summary",
	reasonRetentionBudget:                   "retention-budget-exhausted",
	reasonSummarizingFunction:               "summarizing-function",
	reasonFunctionSummarized:                "function-summarized",
}

// String renders the stable trace code at the output boundary.
func (reason Reason) String() string {
	if int(reason) >= len(reasonCodes) {
		return "invalid-lifecycle-summary-reason"
	}
	return reasonCodes[reason]
}

// Proof retains the underlying evidence and the summary rule, when applicable.
// SummaryReason never replaces an SSA reason with a string from another domain.
type Proof struct {
	ssaflow.Proof
	SummaryReason Reason
}

func (proof Proof) traceReason() string {
	if proof.SummaryReason != reasonNone {
		return proof.SummaryReason.String()
	}
	return proof.Reason.String()
}

// CompletionProof retains path coverage along with its summary explanation.
type CompletionProof struct {
	ssaflow.CompletionProof
	SummaryReason Reason
}
