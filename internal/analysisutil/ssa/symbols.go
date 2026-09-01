package ssautil

import (
	"go/types"

	"github.com/kojah/gohawk/internal/analysisutil"

	"golang.org/x/tools/go/ssa"
)

// CallMatchesSymbol reports whether common statically resolves to symbol.
func CallMatchesSymbol(common *ssa.CallCommon, symbol analysisutil.Symbol) bool {
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

// ValueMatchesSymbol reports whether value is the exact package declaration
// identified by symbol.
func ValueMatchesSymbol(value ssa.Value, symbol analysisutil.Symbol) bool {
	global, ok := value.(*ssa.Global)
	return ok && symbol.MatchesObject(global.Object())
}
