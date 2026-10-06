package ssaflow

import (
	"go/constant"
	"go/token"
	"strconv"
	"strings"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

// Static access paths name a value by the field and constant-index steps that
// select it from a root, rendered as strings such as "field:0/index:1" so a
// summary can carry them across packages. EmbeddedFieldPath is a different
// vocabulary for a different question: the numeric embedded-field indexes a
// promoted method selects from one exact root, bounded and never rendered.
// The two are kept apart because a summary step must survive serialization
// and a promotion path must not be confused with an arbitrary field selection.

// JoinAccessPath renders a static field/index path for a summary or key.
func JoinAccessPath(path []string) string { return strings.Join(path, "/") }

// SplitAccessPath reverses JoinAccessPath.
func SplitAccessPath(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, "/")
}

// AccessPath identifies one SSA value relative to the aggregate root from
// which its fields and indexes are selected.
type AccessPath struct {
	Value ssa.Value
	Root  ssa.Value
}

// ValueIsAccessPathFrom reports whether value is root itself or a statically
// identifiable field or constant-index projection beneath root.
func ValueIsAccessPathFrom(value, root ssa.Value) bool {
	return ValueIsAccessPathFromWithin(value, root, nil)
}

// ValueIsAccessPathFromWithin applies the same projection policy under budget.
// A false result at exhaustion is unavailable, not evidence of unrelated roots.
func ValueIsAccessPathFromWithin(value, root ssa.Value, budget *proofs.SearchBudget) bool {
	_, ok := AccessPathStepsWithin(value, root, budget)
	return ok
}

// SameAccessPathWithin shares both path searches and step comparison with
// budget. Both paths must be nameable; unlike ProveIdentityWithin, direct value
// identity cannot bypass that policy. Cutoff supplies no matching-path evidence.
func SameAccessPathWithin(left, right AccessPath, budget *proofs.SearchBudget) bool {
	leftPath, leftOK := AccessPathStepsWithin(left.Value, left.Root, budget)
	rightPath, rightOK := AccessPathStepsWithin(right.Value, right.Root, budget)
	return leftOK && rightOK && sameAccessPathSteps(leftPath, rightPath, budget)
}

func sameAccessPathSteps(left, right []string, budget *proofs.SearchBudget) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !budget.Spend() || left[index] != right[index] {
			return false
		}
	}
	return !budget.Exhausted()
}

func AccessPathSteps(value, root ssa.Value) ([]string, bool) {
	return AccessPathStepsWithin(value, root, nil)
}

// AccessPathStepsWithin charges projection visits and structural root checks to
// one allowance. Cutoff returns no path; callers inspect budget availability.
// A nil budget retains default field, constant-index, wrapper and load policy.
func AccessPathStepsWithin(value, root ssa.Value, budget *proofs.SearchBudget) ([]string, bool) {
	return accessPathSteps(value, root, map[ssa.Value]bool{}, budget, nil)
}

// AccessPathReadWithin names the same static path and its load nearest root.
// That load snapshots the root's contents before later pointer selections or
// wrappers. A path with no load has a nil read; it supplies no snapshot.
// Cutoff or an unmodeled path publishes neither the path nor a read.
func AccessPathReadWithin(value, root ssa.Value, budget *proofs.SearchBudget) ([]string, *ssa.UnOp, bool) {
	var read *ssa.UnOp
	path, ok := accessPathSteps(value, root, map[ssa.Value]bool{}, budget, &read)
	if !ok || budget.Exhausted() || budget.PoolExhausted() {
		return nil, nil, false
	}
	return path, read, true
}

func accessPathSteps(value, root ssa.Value, seen map[ssa.Value]bool, budget *proofs.SearchBudget, read **ssa.UnOp) ([]string, bool) {
	if !budget.Spend() || value == nil || root == nil || seen[value] {
		return nil, false
	}
	if StructurallyIdenticalWithin(value, root, budget) {
		return nil, true
	}
	if budget.Exhausted() {
		return nil, false
	}
	seen[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return accessPathSteps(inner, root, seen, budget, read)
	}
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		path, ok := accessPathSteps(typed.X, root, seen, budget, read)
		return appendAccess(path, "field:"+strconv.Itoa(typed.Field), ok)
	case *ssa.IndexAddr:
		index, ok := ConstantIndex(typed.Index)
		if !ok {
			return nil, false
		}
		path, baseOK := accessPathSteps(typed.X, root, seen, budget, read)
		return appendAccess(path, "index:"+index, baseOK)
	case *ssa.UnOp:
		if typed.Op == token.MUL && read != nil {
			*read = typed
		}
		if typed.Op == token.MUL && StructurallyIdenticalWithin(typed.X, root, budget) {
			return nil, true
		}
		if budget.Exhausted() {
			return nil, false
		}
		return accessPathSteps(typed.X, root, seen, budget, read)
	}
	return nil, false
}

func appendAccess(path []string, component string, ok bool) ([]string, bool) {
	if !ok {
		return nil, false
	}
	return append(path, component), true
}

func ConstantIndex(value ssa.Value) (string, bool) {
	literal, ok := value.(*ssa.Const)
	if !ok || literal.Value == nil || literal.Value.Kind() != constant.Int {
		return "", false
	}
	return literal.Value.ExactString(), true
}
