package heapmodel

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
)

// Only a value whose type can refer to another object can contain one.
func TestCanHoldReference(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "references", `package references
import "unsafe"
type names struct{ first, last string; age int }
type linked struct{ name string; next *linked }
var (
	text string
	count int
	plain names
	pair [2]names
	next linked
	pointer *int
	anything any
	items []int
	raw unsafe.Pointer
)
`)
	for name, want := range map[string]bool{
		"text": false, "count": false, "plain": false, "pair": false,
		"next": true, "pointer": true, "anything": true, "items": true, "raw": true,
	} {
		if got := CanHoldReference(pkg.Var(name).Type().(*types.Pointer).Elem()); got != want {
			t.Errorf("%s: CanHoldReference = %t, want %t", name, got, want)
		}
	}
}

func TestResultTupleReferenceCapability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "resultreferences", `package resultreferences
func void() {}
func scalar() (int, bool) { return 0, false }
func pointer() (int, *int) { return 0, new(int) }
func boxed() (int, any) { return 0, nil }
func callback() (int, func()) { return 0, func() {} }
func nested() (int, struct{ p *int }) { return 0, struct{ p *int }{} }
func plain() (int, struct{ n int }) { return 0, struct{ n int }{} }
`)
	for name, want := range map[string]bool{
		"void": false, "scalar": false, "plain": false,
		"pointer": true, "boxed": true, "callback": true, "nested": true,
	} {
		if got := CanHoldReference(pkg.Func(name).Signature.Results()); got != want {
			t.Errorf("%s results hold references=%v, want %v", name, got, want)
		}
	}
}

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
