package gohawk

import (
	"github.com/kojah/gohawk/general"
	"golang.org/x/tools/go/analysis"
)

// AnalyzerGroup is a related set of gohawk analyzers.
type AnalyzerGroup = general.AnalyzerGroup

// AnalyzerInfo describes analyzer capabilities used by integrations and documentation.
type AnalyzerInfo = general.AnalyzerInfo

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
