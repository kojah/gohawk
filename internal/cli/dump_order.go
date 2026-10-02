package cli

import (
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/ssa"
)

// Fact and heap dumps order declarations by resolved file positions. Raw
// token positions depend on the fileset's allocation order; filtering and
// collecting the declarations remain the responsibility of each dump.

// functionOrder breaks equal source positions with qualified SSA names,
// retaining deterministic order for functions without distinct positions.
func functionOrder(action *checker.Action) func(left, right *ssa.Function) int {
	return func(left, right *ssa.Function) int {
		if order := comparePositions(action, left.Pos(), right.Pos()); order != 0 {
			return order
		}
		return strings.Compare(left.String(), right.String())
	}
}

// comparePositions orders two positions by file name, then offset, which
// is stable across runs where the raw token.Pos values are not.
func comparePositions(action *checker.Action, left, right token.Pos) int {
	a, b := action.Package.Fset.Position(left), action.Package.Fset.Position(right)
	if a.Filename != b.Filename {
		return strings.Compare(a.Filename, b.Filename)
	}
	return a.Offset - b.Offset
}
