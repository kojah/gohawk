package ssaflow

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
