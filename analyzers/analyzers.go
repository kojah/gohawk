package analyzers

import (
	"github.com/kojah/gohawk/general"
	"golang.org/x/tools/go/analysis"
)

// AnalyzerGroup is a related set of gohawk analyzers.
type AnalyzerGroup = general.AnalyzerGroup

// AnalyzerInfo describes analyzer capabilities used by integrations and documentation.
type AnalyzerInfo = general.AnalyzerInfo

// AnalyzerProfile controls whether an analyzer runs without explicit selection.
type AnalyzerProfile = general.AnalyzerProfile

const (
	AnalyzerProfileDefault = general.AnalyzerProfileDefault
	AnalyzerProfileOptIn   = general.AnalyzerProfileOptIn
)

// AnalyzerTag describes why an analyzer's findings matter.
type AnalyzerTag = general.AnalyzerTag

const (
	AnalyzerTagCorrectness = general.AnalyzerTagCorrectness
	AnalyzerTagReliability = general.AnalyzerTagReliability
	AnalyzerTagPolicy      = general.AnalyzerTagPolicy
)

// AnalyzerGroups returns all gohawk analyzers grouped by concern.
func AnalyzerGroups() []AnalyzerGroup {
	return general.AnalyzerGroups()
}

// AnalyzerMetadata returns documentation metadata keyed by analyzer name.
func AnalyzerMetadata() map[string]AnalyzerInfo {
	return general.AnalyzerMetadata()
}

// Analyzers returns all gohawk analyzers in stable execution order.
func Analyzers() []*analysis.Analyzer {
	return general.Analyzers()
}

// DefaultAnalyzers returns the analyzers enabled when none are selected explicitly.
func DefaultAnalyzers() []*analysis.Analyzer {
	return general.DefaultAnalyzers()
}
