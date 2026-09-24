package heapmodel

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Derivation is the may-relation "source contributes to value": through
// operands, through a local load and store pair, and, since the points-to
// graph answers identity, through any copy, join, or captured cell the
// graph resolves. Its polarity ends the walk at anything it cannot follow.

// ValueDerivesFrom reports whether source contributes to value through SSA
// operands or a local load/store pair. This is a may-relation: every store to
// the loaded address counts, not only the one that reaches the load.
//
// A load through a field or element address also derives from a value stored
// into the enclosing aggregate as a whole, when that aggregate is only ever
// written whole. The builder spills a struct or array parameter, and a copy
// such as k := j, into a local cell before it can select a field, so
// j.out.Close() in func (j job) reaches the parameter only through that
// spill. Without this step a by-value parameter could never be proven closed
// while the same body with a pointer parameter is, because the pointer's
// field address selects from the parameter directly. A cell with a store into
// one of its fields is not crossed: the field a later load returns may be the
// replacement rather than a component of the stored aggregate, and the
// analyzer must keep such a replaced resource reportable.
func ValueDerivesFrom(value, source ssa.Value, seen map[ssa.Value]bool) bool {
	return ssaflow.DerivesFrom(value, source, seen, MayAlias)
}
