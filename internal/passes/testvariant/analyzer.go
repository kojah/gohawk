// Package testvariant provides the prerequisite marker used when an analysis
// driver treats an augmented test package as its canonical package pass.
package testvariant

import (
	"reflect"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
)

// Analyzer marks drivers whose test variant contains the canonical production
// files. Consumers use its result to distinguish those files from duplicate
// production copies emitted by drivers that also analyze the ordinary package.
var Analyzer = &analysis.Analyzer{
	Name:       "gohawk_test_variant_files",
	Doc:        "marks a driver whose test variant is its canonical package pass",
	ResultType: reflect.TypeFor[syntax.CanonicalTestVariant](),
	Run: func(*analysis.Pass) (any, error) {
		return syntax.CanonicalTestVariant{}, nil
	},
}

// IncludeProductionFiles returns an analyzer copy configured for an embedding
// driver that does not also run the ordinary production package. Without this
// marker, production files in the driver's augmented test package would be
// mistaken for duplicate copies and skipped.
func IncludeProductionFiles(analyzer *analysis.Analyzer) *analysis.Analyzer {
	wrapper := *analyzer
	wrapper.Requires = append([]*analysis.Analyzer(nil), analyzer.Requires...)
	wrapper.Requires = append(wrapper.Requires, Analyzer)
	return &wrapper
}
