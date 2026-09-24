package nilargument

// Nil-argument reasons label policy and supporting evidence at the trace
// boundary; they never replace the shared heap proof's own classifications.
type nilArgumentReason uint8

const (
	reasonNone nilArgumentReason = iota
	reasonCalleeDereferences
	reasonSlotTypeUnknown
	reasonSlotNotPointer
	reasonSlotNotProvenNil
	reasonNilSlotDereferenced
	reasonEarlierCallUnsummarized
	reasonEarlierCalls
	nilArgumentReasonCount
)

func (reason nilArgumentReason) String() string {
	switch reason {
	case reasonNone:
		return ""
	case reasonCalleeDereferences:
		return "callee-dereferences-argument"
	case reasonSlotTypeUnknown:
		return "slot-type-unknown"
	case reasonSlotNotPointer:
		return "slot-not-pointer"
	case reasonSlotNotProvenNil:
		return "slot-not-proven-nil"
	case reasonNilSlotDereferenced:
		return "nil-slot-dereferenced"
	case reasonEarlierCallUnsummarized:
		return "earlier-call-unsummarized"
	case reasonEarlierCalls:
		return "earlier-calls"
	default:
		return "invalid-nil-argument-reason"
	}
}
