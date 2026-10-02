package heapmodel

import "go/types"

// By-value type traversal follows struct fields, array elements and SSA tuples.
// Reference edges remain leaves; the caller decides what matching a type means.
func anyByValueType(value types.Type, matches func(types.Type) bool) bool {
	if matches(value) {
		return true
	}
	switch value := value.Underlying().(type) {
	case *types.Struct:
		for field := range value.Fields() {
			if anyByValueType(field.Type(), matches) {
				return true
			}
		}
	case *types.Array:
		return anyByValueType(value.Elem(), matches)
	case *types.Tuple:
		for variable := range value.Variables() {
			if anyByValueType(variable.Type(), matches) {
				return true
			}
		}
	}
	return false
}
