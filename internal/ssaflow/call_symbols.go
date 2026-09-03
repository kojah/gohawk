package ssaflow

import (
	"go/types"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// CallMatchesSymbol reports whether common statically resolves to symbol.
func CallMatchesSymbol(common *ssa.CallCommon, symbol syntax.Symbol) bool {
	if common == nil {
		return false
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return symbol.MatchesObject(types.Universe.Lookup(builtin.Name()))
	}
	if common.Method != nil {
		if symbol.MatchesObject(common.Method) {
			return true
		}
		return symbol.MatchesMethod(common.Method.Name(), common.Value.Type())
	}
	callee := common.StaticCallee()
	if callee == nil {
		return false
	}
	if symbol.MatchesObject(callee.Object()) {
		return true
	}
	receiver := CallReceiver(common)
	return receiver != nil && symbol.MatchesMethod(callee.Name(), receiver.Type())
}

// CallMatchesAnySymbol reports whether common statically resolves to one of symbols.
func CallMatchesAnySymbol(common *ssa.CallCommon, symbols ...syntax.Symbol) bool {
	for _, symbol := range symbols {
		if CallMatchesSymbol(common, symbol) {
			return true
		}
	}
	return false
}

// ValueMatchesSymbol reports whether value is the exact package declaration
// identified by symbol.
func ValueMatchesSymbol(value ssa.Value, symbol syntax.Symbol) bool {
	global, ok := value.(*ssa.Global)
	return ok && symbol.MatchesObject(global.Object())
}

// ValueMatchesAnySymbol reports whether value is one of the exact package declarations.
func ValueMatchesAnySymbol(value ssa.Value, symbols ...syntax.Symbol) bool {
	for _, symbol := range symbols {
		if ValueMatchesSymbol(value, symbol) {
			return true
		}
	}
	return false
}
