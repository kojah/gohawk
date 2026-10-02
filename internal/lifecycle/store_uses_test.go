package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Both queries follow forwarding forms, but only general transfer follows
// reads from a local cell. A return or opaque use is not a field store.
func TestForwardUseTransferBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type holder struct { value *int; boxed any }
var global *int
func consume(*int)
func identity(p *int) *int { return p }
func pair(p *int) (*int, bool) { return p, false }
func directField(p *int, h *holder) { h.value = identity(p) }
func boxedField(p *int, h *holder) { h.boxed = identity(p) }
func tupleField(p *int, h *holder) { h.value, _ = pair(p) }
func returned(p *int) *int { return identity(p) }
func globalStore(p *int) { global = identity(p) }
func opaqueUse(p *int) { consume(identity(p)) }
func localCell(p *int, h *holder) {
	x := identity(p)
	func() { _ = x }()
	h.value = x
}
func localCycle(p *int, n int) {
	x := identity(p)
	for i := 0; i < n; i++ {
		consume(x)
		if i%2 == 0 { x = identity(nil) }
	}
}
func returnedCycle(p *int, n int) *int {
	x := identity(p)
	for i := 0; i < n; i++ { if i%2 == 0 { x = identity(nil) } }
	return x
}
`)
	for _, test := range []struct {
		name            string
		field, transfer bool
	}{
		{"directField", true, true},
		{"boxedField", true, true},
		{"tupleField", true, true},
		{"returned", false, true},
		{"globalStore", false, false},
		{"opaqueUse", false, false},
		{"localCell", false, true},
		{"localCycle", false, false},
		{"returnedCycle", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			if len(calls) == 0 {
				t.Fatal("missing constructor call")
			}
			call := calls[0]
			if got := CallTransfersValueToField(call, function.Params[0]); got != test.field {
				t.Errorf("field transfer = %t, want %t", got, test.field)
			}
			if got := ValueHasTransferUse(call); got != test.transfer {
				t.Errorf("general transfer = %t, want %t", got, test.transfer)
			}
		})
	}
}

func TestFluentTransferPreservesReceiverType(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type owner struct { p *int }
func (o *owner) Next() *owner { return o }
func (o *owner) Read() *int { return o.p }
func retained(o *owner) *owner { return o.Next() }
func borrowed(o *owner) *int { return o.Read() }
func local(o *owner) { o.Next() }
`)
	for name, want := range map[string]bool{"retained": true, "borrowed": false, "local": false} {
		if got := ValueHasTransferUse(pkg.Func(name).Params[0]); got != want {
			t.Errorf("%s: receiver transfer = %t, want %t", name, got, want)
		}
	}
}

func TestCallConsumptionAliasBoundary(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type holder struct{ptr *int;result *int}
 func identity(a,b *int)*int{return b}
 func keepHolder(h *holder)*int{return h.ptr}
 func cleanup(a,b *int)func(){return func(){println(b)}}
 func cleanupHolder(h *holder)func(){return func(){println(h.ptr)}}
 func fieldLate(){p:=new(int);q:=new(int);h:=new(holder);h.result=identity(q,p);println(p,h)}
 func fieldOther(){p:=new(int);q:=new(int);h:=new(holder);h.result=identity(q,q);println(p,h)}
 func fieldContained(){p:=new(int);h:=new(holder);h.result=keepHolder(&holder{ptr:p});println(p,h)}
 func deferredLate(){p:=new(int);q:=new(int);defer cleanup(q,p)();println(p)}
 func deferredOther(){p:=new(int);q:=new(int);defer cleanup(q,q)();println(p)}
 func deferredContained(){p:=new(int);defer cleanupHolder(&holder{ptr:p})();println(p)}
 `)
	for _, test := range []struct {
		name        string
		field, want bool
	}{
		{"fieldLate", true, true},
		{"fieldOther", true, false},
		{"fieldContained", true, false},
		{"deferredLate", false, true},
		{"deferredOther", false, false},
		{"deferredContained", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			target := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			var got bool
			if test.field {
				got = CallTransfersValueToField(call, target)
			} else {
				got = CallReturnsDeferredCleanup(call, target)
			}
			if got != test.want {
				t.Fatalf("consumption=%v want %v", got, test.want)
			}
		})
	}
}
