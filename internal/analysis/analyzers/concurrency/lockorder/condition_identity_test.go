package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const conditionIdentityFixture = `package state
func computed() bool
func direct(flag bool) bool { return flag }
func named(flag namedBool) namedBool { return flag }
type namedBool bool
func number(n int) int { return n }
func once(flag bool) bool { return !flag }
func loaded(flag *bool) bool { return *flag }
func called() bool { return computed() }
func merged(a, b, choose bool) bool { value := a; if choose { value = b }; return value }
func repeated(flag bool) { for computed() { if !flag { break } } }
func equal(a,b int) bool { return a == b }
func unequal(a,b int) bool { return a != b }
func ordered(a,b int) bool { return a < b }
`

func TestConditionIdentityStableBooleanSources(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "state", conditionIdentityFixture)
	for _, test := range []struct {
		name   string
		prefix string
	}{
		{"direct", "boolean:"},
		{"named", "boolean:"},
		{"number", ""},
		{"once", "boolean:"},
		{"loaded", "boolean:"},
		{"called", "boolean:"},
		{"merged", "boolean:"},
		{"equal", "==:"},
		{"unequal", "!=:"},
		{"ordered", ""},
	} {
		fn := pkg.Func(test.name)
		value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
		identity, known := conditionIdentity(value, nil)
		if known != (test.prefix != "") || !strings.HasPrefix(identity, test.prefix) {
			t.Errorf("%s: identity %q, known=%t", test.name, identity, known)
		}
		if test.prefix == "boolean:" && identity != "boolean:"+conditionOperandIdentity(value) {
			t.Errorf("%s: did not preserve exact SSA identity", test.name)
		}
	}
	fn := pkg.Func("repeated")
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if identity, known := conditionIdentity(call, nil); known || identity != "" {
			t.Errorf("cyclic computation became stable: %q/%t", identity, known)
		}
	}
	// The parameter stays stable even when an instruction using it is in a loop.
	if identity, known := conditionIdentity(fn.Params[0], nil); !known || !strings.HasPrefix(identity, "boolean:") {
		t.Errorf("loop parameter: %q/%t", identity, known)
	}
	if identity, known := conditionIdentity(nil, nil); known || identity != "" {
		t.Errorf("nil condition: %q/%t", identity, known)
	}
}
