package heapmodel

import "go/types"

// By-value type traversal follows struct fields and array elements only.
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
	}
	return false
}
