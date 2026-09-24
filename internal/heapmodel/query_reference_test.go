package heapmodel

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
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
