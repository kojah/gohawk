package processownership

// Process ownership reasons classify the rule chosen by the existing proof.
// They become textual trace codes only when an event is emitted.
type processReason uint8

const (
	reasonNone processReason = iota
	reasonHelperOwnershipUnknown
	reasonStartFailureReturn
	reasonImpossibleNilReturn
	reasonReturnedOwner
	reasonReturnedHandle
	reasonReturnedMergedOwner
	reasonWaitOwnershipProven
	reasonUnownedReturn
	reasonAmbiguousWaitOwnership
	processReasonCount
)

func (reason processReason) String() string {
	switch reason {
	case reasonNone:
		return ""
	case reasonHelperOwnershipUnknown:
		return "helper-command-ownership-unknown"
	case reasonStartFailureReturn:
		return "start-failure-return"
	case reasonImpossibleNilReturn:
		return "impossible-nil-process-return"
	case reasonReturnedOwner:
		return "returned-value-owns-command"
	case reasonReturnedHandle:
		return "returns-process-handle"
	case reasonReturnedMergedOwner:
		return "returned-value-owns-merged-command"
	case reasonWaitOwnershipProven:
		return "wait-ownership-proven"
	case reasonUnownedReturn:
		return "unowned-return"
	case reasonAmbiguousWaitOwnership:
		return "ambiguous-wait-ownership"
	default:
		return "invalid-process-reason"
	}
}
