package ssaflow

import (
	"testing"

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
		form    TransparentValueForm
		another TransparentValueForm
	}{
		{name: "change interface", value: &ssa.ChangeInterface{X: operand}, form: TransparentChangeInterface, another: TransparentChangeType},
		{name: "change type", value: &ssa.ChangeType{X: operand}, form: TransparentChangeType, another: TransparentConvert},
		{name: "convert", value: &ssa.Convert{X: operand}, form: TransparentConvert, another: TransparentMakeInterface},
		{name: "make interface", value: &ssa.MakeInterface{X: operand}, form: TransparentMakeInterface, another: TransparentChangeInterface},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unwrapped, ok := UnwrapTransparentValue(test.value, test.form)
			if !ok || unwrapped != operand {
				t.Fatalf("UnwrapTransparentValue() = (%v, %v), want operand and true", unwrapped, ok)
			}
			if unwrapped, ok := UnwrapTransparentValue(test.value, test.another); ok || unwrapped != nil {
				t.Fatalf("UnwrapTransparentValue() with another form = (%v, %v), want nil and false", unwrapped, ok)
			}
		})
	}

	if unwrapped, ok := UnwrapTransparentValue(operand, TransparentChangeType); ok || unwrapped != nil {
		t.Fatalf("UnwrapTransparentValue() on an unwrapped value = (%v, %v), want nil and false", unwrapped, ok)
	}
}
