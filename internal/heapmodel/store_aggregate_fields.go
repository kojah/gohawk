package heapmodel

import (
	"go/types"
	"strconv"
)

// walkStructReferences enumerates bounded reference slots in a by-value struct.
// Pointers stay leaves. Arrays and exhausted limits invoke cut so both snapshot
// construction and summary projection retain unknown evidence beyond the bound.
func walkStructReferences(typ types.Type, visit, cut func(string)) {
	count := 0
	var fields func(types.Type, string, int)
	fields = func(typ types.Type, path string, depth int) {
		if !tracked(typ) {
			return
		}
		if depth > SummaryPaths || count >= SummarySlots {
			cut("")
			return
		}
		switch typed := typ.Underlying().(type) {
		case *types.Struct:
			for index := range typed.NumFields() {
				fields(typed.Field(index).Type(), joinSlotPath(path, "field:"+strconv.Itoa(index)), depth+1)
			}
		case *types.Array:
			cut(path)
		default:
			count++
			visit(path)
		}
	}
	fields(typ, "", 0)
}
