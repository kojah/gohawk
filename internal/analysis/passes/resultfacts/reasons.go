package resultfacts

// Reason explains a result inference boundary. It is separate from Available
// and the guarantees themselves: no boundary does not establish any guarantee.
type Reason uint8

const (
	ReasonNone Reason = iota
	ReasonSummaryUnavailable
	ReasonBodyUnavailable
	ReasonCountLimit
	ReasonBudgetExhausted
	ReasonNoNormalReturnWitness
	reasonCount
)

// String is the stable textual representation used at output boundaries.
func (reason Reason) String() string {
	switch reason {
	case ReasonNone:
		return ""
	case ReasonSummaryUnavailable:
		return "result-summary-unavailable"
	case ReasonBodyUnavailable:
		return "result-body-unavailable"
	case ReasonCountLimit:
		return "result-count-limit"
	case ReasonBudgetExhausted:
		return "result-budget-exhausted"
	case ReasonNoNormalReturnWitness:
		return "result-no-normal-return-witness"
	default:
		return "invalid-result-reason"
	}
}
