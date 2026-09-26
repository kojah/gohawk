package cancellationownership

// Cancellation reasons classify the authoritative obligation proof. Formatting
// belongs at the trace boundary and does not determine the proof's outcome.
type cancellationReason uint8

const (
	reasonCancellationNone cancellationReason = iota
	reasonCancellationUnknown
	reasonCancellationReleased
	reasonCancellationTransferred
	reasonCancellationLost
	// The label reasons name why the classifier labelled one instruction.
	reasonLabelRelease
	reasonLabelTransfer
	reasonLabelOpaqueUse
	reasonLabelParentContextUse
	cancellationReasonCount
)

func (reason cancellationReason) String() string {
	switch reason {
	case reasonCancellationNone:
		return ""
	case reasonCancellationUnknown:
		return "ambiguous-cancellation-use"
	case reasonCancellationReleased:
		return "exact-cancellation-release"
	case reasonCancellationTransferred:
		return "exact-cancellation-transfer"
	case reasonCancellationLost:
		return "unowned-return"
	case reasonLabelRelease:
		return "release"
	case reasonLabelTransfer:
		return "transfer"
	case reasonLabelOpaqueUse:
		return "opaque-cancellation-use"
	case reasonLabelParentContextUse:
		return "parent-context-use"
	default:
		return "invalid-cancellation-reason"
	}
}
