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
