package lifecycle

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestReturnedSliceElementOwnership(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type resource struct{}
func stored(value, other *resource) []*resource {
	values := make([]*resource, 3)
	values[1] = value
	return values
}
func differentElement(value, other *resource) []*resource {
	values := make([]*resource, 3)
	values[1] = other
	return values
}
func differentSlice(value, other *resource) []*resource {
	values := make([]*resource, 3)
	values[1] = value
	return make([]*resource, 3)
}
func dynamic(value, other *resource, index int) []*resource {
	values := make([]*resource, 3)
	values[index] = value
	return values
}
`)
	for _, test := range []struct {
		name string
		owns bool
	}{
		{"stored", true},
		{"differentElement", false},
		{"differentSlice", false},
		{"dynamic", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				_, ok := instruction.(*ssa.Return)
				return ok
			})
			if owns := ReturnedValueOwnsValue(instruction.(*ssa.Return), function.Params[0]); owns != test.owns {
				t.Errorf("ReturnedValueOwnsValue = %t, want %t", owns, test.owns)
			}
		})
	}
}
