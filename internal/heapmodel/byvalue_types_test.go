package heapmodel

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestByValueTypeQueryBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "byvaluetypes", `package byvaluetypes
type owner struct{value int}
type alias=owner
type sameShape struct{value int}
type nested struct{value [2]owner}
type recursive struct{next *recursive}
var (
 root owner
 aliased alias
 same sameShape
 composite nested
 array [2]owner
 empty [0]owner
 pointer *owner
 pointerField struct{value *owner}
 interfaceField struct{value any}
 sliceField struct{value []owner}
 callback func(owner)
 linked recursive
 text string
)
`)
	owner := pkg.Type("owner").Type().Underlying().(*types.Struct)
	for _, test := range []struct {
		name                string
		reference, contains bool
	}{
		{"root", false, true},
		{"aliased", false, true},
		{"same", false, true},
		{"composite", false, true},
		{"array", false, true},
		{"empty", false, true},
		{"pointer", true, false},
		{"pointerField", true, false},
		{"interfaceField", true, false},
		{"sliceField", true, false},
		{"callback", true, false},
		{"linked", true, false},
		{"text", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			typ := pkg.Var(test.name).Type().(*types.Pointer).Elem()
			if got := CanHoldReference(typ); got != test.reference {
				t.Fatalf("reference capability = %v, want %v", got, test.reference)
			}
			if got := holdsByValue(typ, owner); got != test.contains {
				t.Fatalf("by-value overwrite capability = %v, want %v", got, test.contains)
			}
		})
	}
}
