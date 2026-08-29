package analysisutil

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

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
	if IsErrorType(value.Type()) {
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
