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

// TransparentValueForm identifies an SSA wrapper that a caller has chosen to
// treat as preserving the evidence it is following. Convert is deliberately
// opt-in because it may change a value's representation or meaning.
type TransparentValueForm uint8

const (
	TransparentChangeInterface TransparentValueForm = 1 << iota
	TransparentChangeType
	TransparentConvert
	TransparentMakeInterface
)

// UnwrapTransparentValue returns the operand of value only when its concrete
// SSA form is among forms. There is intentionally no catch-all form: each
// analysis must select the transformations that preserve its own evidence.
func UnwrapTransparentValue(value ssa.Value, forms TransparentValueForm) (ssa.Value, bool) {
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return transparentOperand(typed.X, forms, TransparentChangeInterface)
	case *ssa.ChangeType:
		return transparentOperand(typed.X, forms, TransparentChangeType)
	case *ssa.Convert:
		return transparentOperand(typed.X, forms, TransparentConvert)
	case *ssa.MakeInterface:
		return transparentOperand(typed.X, forms, TransparentMakeInterface)
	default:
		return nil, false
	}
}

func transparentOperand(operand ssa.Value, forms, form TransparentValueForm) (ssa.Value, bool) {
	if forms&form == 0 {
		return nil, false
	}
	return operand, true
}

// ValueSources returns error identities contributing to value. Non-error
// observations may derive from their operands, but an error-producing call is
// a new identity unless it is an exactly modeled wrapper.
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
		collectErrorIdentitySources(value, sources, seen)
		return
	}
	switch typed := value.(type) {
	case *ssa.Call:
		if typed.Common().IsInvoke() {
			collectValueSources(typed.Common().Value, sources, seen)
		}
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

// ValueIsAccessPathFrom reports whether value is root itself or a statically
// identifiable field or constant-index projection beneath root.
func ValueIsAccessPathFrom(value, root ssa.Value) bool {
	_, ok := accessPath(value, root, map[ssa.Value]bool{})
	return ok
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
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return accessPath(inner, root, seen)
	}
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
