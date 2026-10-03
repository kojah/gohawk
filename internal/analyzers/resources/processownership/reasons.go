package processownership

// Process ownership reasons classify the rule chosen by the existing proof.
// They become textual trace codes only when an event is emitted.
type processReason uint8

const (
	reasonNone processReason = iota
	reasonHelperOwnershipUnknown
	reasonStartFailureReturn
	reasonSuccessfulStartProcessNonNil
	reasonReturnedOwner
	reasonReturnedHandle
	reasonReturnedMergedOwner
	reasonWaitOwnershipProven
	reasonUnownedReturn
	reasonAmbiguousWaitOwnership
	reasonUnusedCommandOwnershipUnknown
	reasonProgramLifetimeOwnershipUnknown
	reasonCommandUseCutoff
	reasonCallerCommandOwnershipUnknown
	reasonAggregateCommandOwnershipUnknown
	reasonPreStartOwnershipUnknown
	reasonSuccessfulStartCannotReturn
	reasonLocalWaitObligation
	reasonPreStartCutoff
	reasonPreStartEvidenceUnavailable
	processReasonCount
)

// Presentation cannot choose ownership policy. This read-only table has one
// spelling per proof reason; tests require every enum entry to be covered.
var processReasonCodes = [...]string{
	reasonNone:                             "",
	reasonHelperOwnershipUnknown:           "helper-command-ownership-unknown",
	reasonStartFailureReturn:               "start-failure-return",
	reasonSuccessfulStartProcessNonNil:     "successful-start-process-non-nil",
	reasonReturnedOwner:                    "returned-value-owns-command",
	reasonReturnedHandle:                   "returns-process-handle",
	reasonReturnedMergedOwner:              "returned-value-owns-merged-command",
	reasonWaitOwnershipProven:              "wait-ownership-proven",
	reasonUnownedReturn:                    "unowned-return",
	reasonAmbiguousWaitOwnership:           "ambiguous-wait-ownership",
	reasonUnusedCommandOwnershipUnknown:    "unused-command-ownership-unknown",
	reasonProgramLifetimeOwnershipUnknown:  "program-lifetime-ownership-unknown",
	reasonCommandUseCutoff:                 "command-use-budget-exhausted",
	reasonCallerCommandOwnershipUnknown:    "caller-command-ownership-unknown",
	reasonAggregateCommandOwnershipUnknown: "aggregate-command-ownership-unknown",
	reasonPreStartOwnershipUnknown:         "pre-start-ownership-unknown",
	reasonSuccessfulStartCannotReturn:      "successful-start-cannot-return",
	reasonLocalWaitObligation:              "local-wait-obligation",
	reasonPreStartCutoff:                   "budget-exhausted",
	reasonPreStartEvidenceUnavailable:      "evidence-unavailable",
}

func (reason processReason) String() string {
	if int(reason) >= len(processReasonCodes) {
		return "invalid-process-reason"
	}
	return processReasonCodes[reason]
}
