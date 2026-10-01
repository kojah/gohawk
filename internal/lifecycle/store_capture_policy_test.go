package lifecycle

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestStoredCallbackOwnershipBoundary(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type resource struct{ value int }
type holder struct { callback func(); value *resource; nested *holder }
func direct(target *holder, value, other *resource) {
	target.callback = func() { _ = value }
}
func unrelated(target *holder, value, other *resource) {
	target.callback = func() { _ = other }
}
func nestedCallback(target *holder, value, other *resource) {
	callback := func() { _ = value }
	target.callback = func() { callback() }
}
func capturedAggregate(target *holder, value, other *resource) {
	owner := &holder{value: value}
	target.callback = func() { _ = owner.value }
}
func storedAggregate(target *holder, value, other *resource) {
	target.nested = &holder{value: value}
}
`)
	for _, test := range []struct {
		name              string
		callback, broader bool
	}{
		{"direct", true, true},
		{"unrelated", false, false},
		{"nestedCallback", true, true},
		{"capturedAggregate", false, true},
		{"storedAggregate", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				store, ok := instruction.(*ssa.Store)
				if !ok {
					return false
				}
				field, ok := store.Addr.(*ssa.FieldAddr)
				return ok && field.X == function.Params[0]
			})
			value := function.Params[1]
			if owns := StoresOwnerOfValueInField(instruction, value); owns != test.callback {
				t.Errorf("StoresOwnerOfValueInField = %t, want %t", owns, test.callback)
			}
			if contains := MayContainValue(instruction.(*ssa.Store).Val, value); contains != test.broader {
				t.Errorf("MayContainValue = %t, want %t", contains, test.broader)
			}
		})
	}
}
