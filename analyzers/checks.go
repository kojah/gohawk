package analyzers

// AnalyzerCheck identifies one independently configurable diagnostic rule.
type AnalyzerCheck string

// CheckKind describes the semantic claim made by a diagnostic rule.
type CheckKind string

const (
	// CheckKindDefect identifies behavior that the available evidence establishes as broken or ineffective.
	CheckKindDefect CheckKind = "defect"
	// CheckKindHazard identifies risky behavior whose harm depends on a wider runtime contract.
	CheckKindHazard CheckKind = "hazard"
	// CheckKindPolicy identifies valid Go that violates an intentionally selected engineering convention.
	CheckKindPolicy CheckKind = "policy"
)

// AnalyzerCheckInfo describes a specific diagnostic rule.
type AnalyzerCheckInfo struct {
	ID    AnalyzerCheck
	Doc   string
	Kind  CheckKind
	OptIn bool
}

// EnabledByDefault reports whether the check runs when its analyzer is selected.
func (info AnalyzerCheckInfo) EnabledByDefault() bool {
	return !info.OptIn
}
