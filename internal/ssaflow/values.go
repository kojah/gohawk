package ssaflow

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Value evidence normalizes SSA wrappers and proves sources, derivation, and
// access-path identity. It follows only modeled value forms; an unfamiliar or
// ambiguous transformation ends the proof instead of guessing equivalence.

func wrappedValue(value ssa.Value) (ssa.Value, bool) {
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return typed.X, true
	case *ssa.ChangeType:
		return typed.X, true
	case *ssa.Convert:
		return typed.X, true
	case *ssa.MakeInterface:
		return typed.X, true
	default:
		return nil, false
	}
}

// ValueSources returns error-bearing SSA values contributing to value.
func ValueSources(value ssa.Value) map[ssa.Value]bool {
	sources := map[ssa.Value]bool{}
	collectValueSources(value, sources, map[ssa.Value]bool{})
	return sources
}

func collectValueSources(value ssa.Value, sources, seen map[ssa.Value]bool) {
	if value == nil || seen[value] {
		return
	}
	seen[value] = true
	if syntax.IsErrorType(value.Type()) {
		sources[value] = true
	}
	switch typed := value.(type) {
	case *ssa.Call:
		for _, argument := range typed.Common().Args {
			collectValueSources(argument, sources, seen)
		}
		return
	case *ssa.Parameter, *ssa.FreeVar:
		return
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil {
			collectValueSources(*operand, sources, seen)
		}
	}
	collectStoredSources(value, sources, seen, map[ssa.Value]bool{})
}

func collectStoredSources(address ssa.Value, sources, seen, memorySeen map[ssa.Value]bool) {
	// Variadic logging and wrapping calls lower arguments into temporary arrays.
	// Following stores recovers original error value instead of losing identity.
	if address == nil || memorySeen[address] || address.Referrers() == nil {
		return
	}
	memorySeen[address] = true
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			collectValueSources(typed.Val, sources, seen)
		case *ssa.FieldAddr:
			collectStoredSources(typed, sources, seen, memorySeen)
		case *ssa.IndexAddr:
			collectStoredSources(typed, sources, seen, memorySeen)
		}
	}
}

// ValuesShareErrorSource reports whether values derive from one error-bearing SSA value.
func ValuesShareErrorSource(left, right ssa.Value) bool {
	leftSources := ValueSources(left)
	for source := range ValueSources(right) {
		if leftSources[source] {
			return true
		}
	}
	return false
}

// SameAsAny reports whether value aliases any candidate.
func SameAsAny(value ssa.Value, candidates []ssa.Value) bool {
	for _, candidate := range candidates {
		if SameValue(value, candidate) {
			return true
		}
	}
	return false
}

// ReturnedSameAsAny reports whether a return transfers any candidate value.
func ReturnedSameAsAny(returned *ssa.Return, candidates []ssa.Value) bool {
	for _, result := range returned.Results {
		if SameAsAny(result, candidates) {
			return true
		}
	}
	return false
}

// ReturnSameValue reports whether a return transfers value.
func ReturnSameValue(returned *ssa.Return, value ssa.Value) bool {
	for _, result := range returned.Results {
		if SameValue(result, value) {
			return true
		}
	}
	return false
}

// CallResult returns the selected SSA result of call. A negative index denotes
// a single-result call represented by the call instruction itself.
func CallResult(call *ssa.Call, index int) ssa.Value { //nolint:ireturn // SSA call results have several concrete forms.
	if index < 0 {
		return call
	}
	if call.Referrers() == nil {
		return nil
	}
	for _, reference := range *call.Referrers() {
		if extract, ok := reference.(*ssa.Extract); ok && extract.Index == index {
			return extract
		}
	}
	return nil
}

// ValueDerivesFrom reports whether source contributes to value through SSA
// operands or a local load/store pair.
func ValueDerivesFrom(value, source ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || source == nil || seen[value] {
		return false
	}
	if SameValue(value, source) {
		return true
	}
	seen[value] = true
	if load, ok := value.(*ssa.UnOp); ok && load.X.Referrers() != nil {
		for _, reference := range *load.X.Referrers() {
			if store, storeOK := reference.(*ssa.Store); storeOK && ValueDerivesFrom(store.Val, source, seen) {
				return true
			}
		}
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	var operands []*ssa.Value
	for _, operand := range instruction.Operands(operands) {
		if operand != nil && ValueDerivesFrom(*operand, source, seen) {
			return true
		}
	}
	return false
}

// AccessPath identifies one SSA value relative to the aggregate root from
// which its fields and indexes are selected.
type AccessPath struct {
	Value ssa.Value
	Root  ssa.Value
}

// SameAccessPath reports whether left and right select the same sequence of
// fields and constant indexes from their respective roots. It maps a closure's
// free-variable access back to the captured binding without equating either
// selected field with the aggregate that contains it.
func SameAccessPath(left, right AccessPath) bool {
	leftPath, leftOK := accessPath(left.Value, left.Root, map[ssa.Value]bool{})
	rightPath, rightOK := accessPath(right.Value, right.Root, map[ssa.Value]bool{})
	if !leftOK || !rightOK || len(leftPath) != len(rightPath) {
		return false
	}
	for index := range leftPath {
		if leftPath[index] != rightPath[index] {
			return false
		}
	}
	return true
}

func accessPath(value, root ssa.Value, seen map[ssa.Value]bool) ([]string, bool) {
	if value == nil || root == nil || seen[value] {
		return nil, false
	}
	if SameValue(value, root) {
		return nil, true
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		path, ok := accessPath(typed.X, root, seen)
		return appendAccess(path, "field:"+strconv.Itoa(typed.Field), ok)
	case *ssa.IndexAddr:
		index, ok := constantIndex(typed.Index)
		if !ok {
			return nil, false
		}
		path, baseOK := accessPath(typed.X, root, seen)
		return appendAccess(path, "index:"+index, baseOK)
	case *ssa.UnOp:
		if typed.Op == token.MUL && SameValue(typed.X, root) {
			return nil, true
		}
		return accessPath(typed.X, root, seen)
	case *ssa.ChangeInterface:
		return accessPath(typed.X, root, seen)
	case *ssa.ChangeType:
		return accessPath(typed.X, root, seen)
	case *ssa.Convert:
		return accessPath(typed.X, root, seen)
	case *ssa.MakeInterface:
		return accessPath(typed.X, root, seen)
	}
	return nil, false
}

func appendAccess(path []string, component string, ok bool) ([]string, bool) {
	if !ok {
		return nil, false
	}
	return append(path, component), true
}

func constantIndex(value ssa.Value) (string, bool) {
	literal, ok := value.(*ssa.Const)
	if !ok || literal.Value == nil || literal.Value.Kind() != constant.Int {
		return "", false
	}
	return literal.Value.ExactString(), true
}

// SuccessBranch reports whether successor is the branch where errorValue is
// nil, when block ends in a recognizable nil comparison.
func SuccessBranch(block, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	comparesErrorToNil := ValueDerivesFrom(comparison.X, errorValue, map[ssa.Value]bool{}) && DefinitelyNil(comparison.Y) ||
		ValueDerivesFrom(comparison.Y, errorValue, map[ssa.Value]bool{}) && DefinitelyNil(comparison.X)
	if !comparesErrorToNil {
		return false, false
	}
	trueBranch := successor == block.Succs[0]
	if comparison.Op == token.EQL {
		return trueBranch, true
	}
	return !trueBranch, true
}

// FunctionFile returns source file containing function.
func FunctionFile(pass *analysis.Pass, function *ssa.Function) *ast.File {
	for _, file := range pass.Files {
		if file.Pos() <= function.Pos() && function.Pos() <= file.End() {
			return file
		}
	}
	return nil
}

// ChannelType reports whether value has channel type.
func ChannelType(value ssa.Value) bool {
	if value == nil {
		return false
	}
	return channelTypeForType(value.Type())
}

func channelTypeForType(value types.Type) bool {
	_, ok := value.Underlying().(*types.Chan)
	return ok
}
