package analyzers

// AnalyzerCheck identifies one independently configurable diagnostic rule.
type AnalyzerCheck string

// AnalyzerCheckInfo describes a specific diagnostic rule.
type AnalyzerCheckInfo struct {
	ID    AnalyzerCheck
	Doc   string
	OptIn bool
}

// EnabledByDefault reports whether the check runs when its analyzer is selected.
func (info AnalyzerCheckInfo) EnabledByDefault() bool {
	return !info.OptIn
}
