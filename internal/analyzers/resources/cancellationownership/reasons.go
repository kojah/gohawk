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
	default:
		return "invalid-cancellation-reason"
	}
}
