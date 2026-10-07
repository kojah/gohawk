package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestDefiniteIdentityRejectsPossibleAliases(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest
func chosen(a, b *int, pick bool) *int {
	v := a
	if pick { v = b }
	return v
}
func loaded(a, b *int, pick bool) *int {
	v := a
	p := &v
	if pick { *p = b }
	return *p
}
func alias(a *int) *int { return a }
func wrapped(a chan int) chan<- int { return a }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"chosen", false}, {"loaded", false}, {"alias", true}, {"wrapped", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			ret := findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
				_, ok := i.(*ssa.Return)
				return ok
			}).(*ssa.Return)
			value, target := ret.Results[0], fn.Params[0]
			if !heapmodel.MayAlias(value, target) {
				t.Fatal("fixture must exercise the possible-identity matcher")
			}
			for _, pair := range [][2]ssa.Value{{value, target}, {target, value}} {
				if got := heapmodel.DefinitelySameValue(pair[0], pair[1]); got != test.want {
					t.Errorf("DefinitelySameValue(%s, %s) = %t, want %t", pair[0], pair[1], got, test.want)
				}
				proof := ssaflow.ProveIdentityWithin(ssaflow.AccessPath{Value: pair[0]}, ssaflow.AccessPath{Value: pair[1]}, nil)
				if proof.Proven() != test.want {
					t.Errorf("identity proof = %#v, want proven %t", proof, test.want)
				}
			}
		})
	}
}

func TestDefiniteIdentityPhiAgreement(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest; func input(a, b *int) {}`)
	a, b := pkg.Func("input").Params[0], pkg.Func("input").Params[1]
	for _, test := range []struct {
		name string
		phi  *ssa.Phi
		want bool
	}{
		{"all agree", &ssa.Phi{Edges: []ssa.Value{a, a}}, true},
		{"mixed", &ssa.Phi{Edges: []ssa.Value{a, b}}, false},
		{"empty", &ssa.Phi{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := heapmodel.DefinitelySameValue(test.phi, a); got != test.want {
				t.Errorf("got %t, want %t", got, test.want)
			}
		})
	}
	cycle := &ssa.Phi{}
	cycle.Edges = []ssa.Value{a, cycle}
	if heapmodel.DefinitelySameValue(cycle, a) {
		t.Fatal("a cyclic phi must not establish identity")
	}
}

func TestCompletionDoesNotPromotePossibleIdentity(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func helper(r *resource) { r.Close() }
func chosen(a, b *resource, pick bool) {
	v := a
	if pick { v = b }
	helper(v)
}
func invoke(fn func()) { fn() }
func callback(a, b func(), pick bool) {
	fn := a
	if pick { fn = b }
	invoke(fn)
}
`)
	fn := pkg.Func("chosen")
	call := findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
		return ssaflow.CallName(ssaflow.InstructionCall(i)) == "helper"
	})
	proof := ProveCompletion(CompletionRequest{
		Instruction: call, Target: fn.Params[0], Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1000),
	})
	if proof.State != proofs.EvidenceUnknown {
		t.Errorf("mixed receiver completion = %#v, want unknown", proof)
	}
	fn = pkg.Func("callback")
	call = findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
		return ssaflow.CallName(ssaflow.InstructionCall(i)) == "invoke"
	})
	if CallInvokesArgumentOnEveryReturn(call, fn.Params[0]) {
		t.Fatal("invoking a selected callback does not guarantee invoking parameter 0")
	}
}

func TestEmbeddedFieldPathRoots(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type Inner struct { Value int }
type Outer struct { Part Inner }
func shared(owner *Outer) *int { return &owner.Part.Value }
func fresh() *int { owner := new(Outer); return &owner.Part.Value }
func snapshots(cell **Outer, replacement *Outer) (*int, *int) {
 first := &(*cell).Part.Value
 *cell = replacement
 return first, &(*cell).Part.Value
}
func ambiguous(a, b *Outer, flag bool) *int { owner := a; if flag { owner = b }; return &owner.Part.Value }
`)
	acceptRoot := func(value ssa.Value) bool {
		switch value.(type) {
		case *ssa.Parameter, *ssa.Alloc, *ssa.UnOp:
			return true
		}
		return false
	}
	resolve := func(value ssa.Value) (ssaflow.EmbeddedFieldPath, bool) {
		return ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value, acceptRoot)
	}
	for _, name := range []string{"shared", "fresh"} {
		function := pkg.Func(name)
		returned := ssaflow.InstructionsOf[*ssa.Return](function)[0]
		path, known := resolve(returned.Results[0])
		if !known || path.Depth != 2 || path.Fields[0] != 0 || path.Fields[1] != 0 {
			t.Fatalf("%s lost embedded path: %+v/%t", name, path, known)
		}
		_, allocated := path.Root.(*ssa.Alloc)
		if allocated != (name == "fresh") {
			t.Errorf("%s confused shared and fresh roots: %T", name, path.Root)
		}
	}
	returned := ssaflow.InstructionsOf[*ssa.Return](pkg.Func("snapshots"))[0]
	first, firstKnown := resolve(returned.Results[0])
	second, secondKnown := resolve(returned.Results[1])
	if !firstKnown || !secondKnown || first.Root == second.Root || first.Root == pkg.Func("snapshots").Params[0] {
		t.Fatalf("loaded snapshots were conflated with each other or their cell: %+v, %+v", first, second)
	}
	if _, known := resolve(ssaflow.InstructionsOf[*ssa.Return](pkg.Func("ambiguous"))[0].Results[0]); known {
		t.Error("different reaching roots must not resolve to one path")
	}
}

func TestEmbeddedFieldPathBoundAndForms(t *testing.T) {
	root := &ssa.Parameter{}
	acceptRoot := func(value ssa.Value) bool { return value == root }
	var value ssa.Value = root
	for depth := 1; depth <= 9; depth++ {
		value = &ssa.FieldAddr{X: value, Field: 0}
		path, known := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value, acceptRoot)
		if known != (depth <= 8) || known && path.Depth != depth {
			t.Fatalf("depth %d: %+v/%t", depth, path, known)
		}
	}
	wrapped := &ssa.ChangeType{X: root}
	if _, known := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), wrapped, acceptRoot); known {
		t.Error("unselected wrapper must remain opaque")
	}
	selected := ssaflow.NewReachingWalk(ssaflow.TransparentChangeType)
	if path, known := ssaflow.ResolveEmbeddedFieldPath(selected, wrapped, acceptRoot); !known || path.Root != root {
		t.Error("selected wrapper lost its root")
	}
	if _, known := (ssaflow.EmbeddedFieldPath{Root: root}).Append(-1); known {
		t.Error("negative field indexes must be rejected")
	}
}

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
		load, ok := returnedValue(t, function).(*ssa.UnOp)
		if !ok {
			t.Fatal("expected returned load")
		}
		if got := heapmodel.ValueDerivesFrom(load, function.Params[0]); got != want {
			t.Errorf("%s: ValueDerivesFrom(returned load, parameter) = %t, want %t", name, got, want)
		}
	}
}
