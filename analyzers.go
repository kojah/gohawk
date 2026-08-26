package gohawk

import (
	"github.com/kojah/gohawk/general"
	"golang.org/x/tools/go/analysis"
)

// Analyzers returns all GoHawk analyzers in stable execution order.
func Analyzers() []*analysis.Analyzer {
	return general.Analyzers()
}
