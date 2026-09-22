package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

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
	resolve := func(value ssa.Value) (EmbeddedFieldPath, bool) {
		return ResolveEmbeddedFieldPath(NewReachingWalk(TransparentNone), value, acceptRoot)
	}
	for _, name := range []string{"shared", "fresh"} {
		function := pkg.Func(name)
		returned := InstructionsOf[*ssa.Return](function)[0]
		path, known := resolve(returned.Results[0])
		if !known || path.Depth != 2 || path.Fields[0] != 0 || path.Fields[1] != 0 {
			t.Fatalf("%s lost embedded path: %+v/%t", name, path, known)
		}
		_, allocated := path.Root.(*ssa.Alloc)
		if allocated != (name == "fresh") {
			t.Errorf("%s confused shared and fresh roots: %T", name, path.Root)
		}
	}
	returned := InstructionsOf[*ssa.Return](pkg.Func("snapshots"))[0]
	first, firstKnown := resolve(returned.Results[0])
	second, secondKnown := resolve(returned.Results[1])
	if !firstKnown || !secondKnown || first.Root == second.Root || first.Root == pkg.Func("snapshots").Params[0] {
		t.Fatalf("loaded snapshots were conflated with each other or their cell: %+v, %+v", first, second)
	}
	if _, known := resolve(InstructionsOf[*ssa.Return](pkg.Func("ambiguous"))[0].Results[0]); known {
		t.Error("different reaching roots must not resolve to one path")
	}
}

func TestEmbeddedFieldPathBoundAndForms(t *testing.T) {
	root := &ssa.Parameter{}
	acceptRoot := func(value ssa.Value) bool { return value == root }
	var value ssa.Value = root
	for depth := 1; depth <= 9; depth++ {
		value = &ssa.FieldAddr{X: value, Field: 0}
		path, known := ResolveEmbeddedFieldPath(NewReachingWalk(TransparentNone), value, acceptRoot)
		if known != (depth <= 8) || known && path.Depth != depth {
			t.Fatalf("depth %d: %+v/%t", depth, path, known)
		}
	}
	wrapped := &ssa.ChangeType{X: root}
	if _, known := ResolveEmbeddedFieldPath(NewReachingWalk(TransparentNone), wrapped, acceptRoot); known {
		t.Error("unselected wrapper must remain opaque")
	}
	if path, known := ResolveEmbeddedFieldPath(NewReachingWalk(TransparentChangeType), wrapped, acceptRoot); !known || path.Root != root {
		t.Error("selected wrapper lost its root")
	}
	if _, known := (EmbeddedFieldPath{Root: root}).Append(-1); known {
		t.Error("negative field indexes must be rejected")
	}
}
