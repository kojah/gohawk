// Package testvariant provides the prerequisite marker used when an analysis
// driver treats an augmented test package as its canonical package pass.
package testvariant

import (
	"reflect"

	"golang.org/x/tools/go/analysis"
)

// Analyzer marks drivers whose test variant contains the canonical production
// files. Consumers use its result to distinguish those files from duplicate
// production copies emitted by drivers that also analyze the ordinary package.
var Analyzer = &analysis.Analyzer{
	Name:       "gohawk_test_variant_files",
	Doc:        "marks a driver whose test variant is its canonical package pass",
	ResultType: reflect.TypeFor[bool](),
	Run: func(*analysis.Pass) (any, error) {
		return true, nil
	},
}
