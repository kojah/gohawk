package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// An opaque read of an owner slot can still select a field of its previous
// occupant. The relationship is possible identity, never a cleanup guarantee.
func TestPlaceholderProjectionAliases(t *testing.T) {
	for _, test := range []struct {
		name, left, right string
		aliases           bool
	}{
		{"field", "&value.mu", "&value.mu", true},
		{"sibling", "&value.mu", "&value.other", false},
		{"nested", "&value.child.mu", "&value.child.mu", true},
		{"nestedSibling", "&value.child.mu", "&value.child.other", false},
		{"element", "&value.items[0]", "&value.items[0]", true},
		{"otherElement", "&value.items[0]", "&value.items[1]", false},
		{"ownerIsNotField", "value", "&value.mu", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "placeholderprobe", `package placeholderprobe
import "sync"
type child struct { mu, other sync.Mutex }
type owner struct { mu, other sync.Mutex; child child; items [2]sync.Mutex }
func opaque(**owner)
func observe(any, any) {}
func body(value *owner) {
 before := `+test.left+`
 opaque(&value)
 observe(before, `+test.right+`)
}
`)
			calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("body"))
			args := calls[len(calls)-1].Common().Args
			proof := ProveMayAlias(args[0], args[1])
			if proof.Aliases != test.aliases {
				t.Errorf("possible placeholder projection = %+v, want aliases %t", proof, test.aliases)
			}
			if DefinitelySameValue(args[0], args[1]) {
				t.Error("opaque owner projection established exact identity")
			}
		})
	}
}
