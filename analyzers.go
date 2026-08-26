package gohawk

import (
	"github.com/kojah/gohawk/general"
	"golang.org/x/tools/go/analysis"
)

// AnalyzerGroup is a related set of GoHawk analyzers.
type AnalyzerGroup = general.AnalyzerGroup

// AnalyzerGroups returns all GoHawk analyzers grouped by concern.
func AnalyzerGroups() []AnalyzerGroup {
	return general.AnalyzerGroups()
}

// Analyzers returns all GoHawk analyzers in stable execution order.
func Analyzers() []*analysis.Analyzer {
	return general.Analyzers()
}
