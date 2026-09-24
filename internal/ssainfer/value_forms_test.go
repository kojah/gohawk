package ssainfer

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestUnwrapTransparentValueRequiresSelectedForm(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

func source(value int) int { return value }
`)
	operand := pkg.Func("source").Params[0]
	tests := []struct {
		name    string
		value   ssa.Value
		form    ssaflow.TransparentValueForm
		another ssaflow.TransparentValueForm
	}{
		{name: "change interface", value: &ssa.ChangeInterface{X: operand}, form: ssaflow.TransparentChangeInterface, another: ssaflow.TransparentChangeType},
		{name: "change type", value: &ssa.ChangeType{X: operand}, form: ssaflow.TransparentChangeType, another: ssaflow.TransparentConvert},
		{name: "convert", value: &ssa.Convert{X: operand}, form: ssaflow.TransparentConvert, another: ssaflow.TransparentMakeInterface},
		{name: "make interface", value: &ssa.MakeInterface{X: operand}, form: ssaflow.TransparentMakeInterface, another: ssaflow.TransparentChangeInterface},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unwrapped, ok := ssaflow.UnwrapTransparentValue(test.value, test.form)
			if !ok || unwrapped != operand {
				t.Fatalf("UnwrapTransparentValue() = (%v, %v), want operand and true", unwrapped, ok)
			}
			if unwrapped, ok := ssaflow.UnwrapTransparentValue(test.value, test.another); ok || unwrapped != nil {
				t.Fatalf("UnwrapTransparentValue() with another form = (%v, %v), want nil and false", unwrapped, ok)
			}
		})
	}

	if unwrapped, ok := ssaflow.UnwrapTransparentValue(operand, ssaflow.TransparentChangeType); ok || unwrapped != nil {
		t.Fatalf("UnwrapTransparentValue() on an unwrapped value = (%v, %v), want nil and false", unwrapped, ok)
	}
}

// A by-value aggregate parameter is spilled into a local cell before a field
// is selected from it, and so is a local copy. A field loaded out of that
// cell derives from the parameter. The cell is crossed only while it is
// written whole and otherwise read: a store into one field, or the cell's
// address reaching a call, ends the derivation, and a field of an unrelated
// local never derives.
func TestValueDerivesFromFollowsAggregateSpill(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type box struct{ value *int; other *int }

func spilled(b box) *int { return b.value }
func copied(b box) *int { k := b; return k.value }
func indexed(b [2]*int) *int { return b[1] }
func overwritten(b box, other *int) *int { b.value = other; return b.value }
func escaped(b box, sink func(*box)) *int { sink(&b); return b.value }
func unrelated(b box, other *int) *int { var k box; k.value = other; return k.value }
`)
	want := map[string]bool{"spilled": true, "copied": true, "indexed": true, "overwritten": false, "escaped": false, "unrelated": false}
	for name, want := range want {
		function := pkg.Func(name)
		load := returnedLoad(t, function)
		if got := ValueDerivesFrom(load, function.Params[0], map[ssa.Value]bool{}); got != want {
			t.Errorf("%s: ValueDerivesFrom(returned load, parameter) = %t, want %t", name, got, want)
		}
	}
}
