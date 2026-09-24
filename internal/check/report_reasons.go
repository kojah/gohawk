package check

// ReportingReason classifies the reporting pipeline, not analyzer policy.
// The producer and CLI filter share these codes without owning each other's
// tracing vocabulary or converting classifications to strings internally.
type ReportingReason uint8

const (
	ReportingNone ReportingReason = iota
	ReportingCandidate
	ReportingDisabled
	ReportingEmitted
	reportingReasonCount
)

// String is the stable trace representation of a reporting classification.
func (reason ReportingReason) String() string {
	switch reason {
	case ReportingNone:
		return ""
	case ReportingCandidate:
		return "diagnostic-candidate"
	case ReportingDisabled:
		return "check-disabled"
	case ReportingEmitted:
		return "diagnostic-reported"
	default:
		return "invalid-reporting-reason"
	}
}
