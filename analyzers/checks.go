package analyzers

// AnalyzerCheck identifies one independently taggable diagnostic rule.
type AnalyzerCheck string

// CheckProfile controls whether a check runs whenever its analyzer is selected.
type CheckProfile string

const (
	CheckProfileDefault CheckProfile = "default"
	CheckProfileOptIn   CheckProfile = "opt-in"
)

// AnalyzerCheckInfo describes why a specific diagnostic rule matters.
type AnalyzerCheckInfo struct {
	ID      AnalyzerCheck
	Doc     string
	Profile CheckProfile
	Tags    []AnalyzerTag
}

// EnabledByDefault reports whether the check runs when its analyzer is selected.
func (info AnalyzerCheckInfo) EnabledByDefault() bool {
	return info.Profile == CheckProfileDefault
}
