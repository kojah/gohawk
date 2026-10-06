package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestBuiltinExecutionDoesNotPreserveReplacedContents(t *testing.T) {
	for _, test := range []struct {
		name, operation string
		private         bool
	}{
		{"directCopy", `copy(dst[:],src[:])`, true},
		{"deferredCopy", `defer copy(dst[:],src[:])`, true},
		{"directClear", `clear(dst[:])`, true},
		{"deferredClear", `defer clear(dst[:])`, true},
		{"asyncCopy", `go copy(dst[:],src[:])`, false},
		{"asyncClear", `go clear(dst[:])`, false},
		{"conditionalCopy", `if flag{defer copy(dst[:],src[:])}`, false},
		{"conditionalClear", `if flag{defer clear(dst[:])}`, false},
		{"repeatedCopy", `for i:=0;i<2;i++{defer copy(dst[:],src[:])}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "builtinexecution", `package builtinexecution
func probe(flag bool)(**int,*int){
 old:=new(int);replacement:=new(int)
 dst:=[1]*int{old};src:=[1]*int{replacement}
 _=src;`+test.operation+`
 return &dst[0],old
}`)
			fn := pkg.Func("probe")
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
				if returned.Block() == fn.Recover {
					continue
				}
				address, old := returned.Results[0], returned.Results[1]
				if value, exact := ContentValue(address, returned); exact && DefinitelySame(value, old) {
					t.Fatal("mutating builtin preserved exact old contents")
				}
				object, known := ExclusiveAt(address, returned)
				if private := known && object.Local; private != test.private {
					t.Fatalf("private=%v known=%v want %v", private, known, test.private)
				}
				return
			}
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Fatalf("missing normal observation:\n%s", dump.String())
		})
	}
}

func TestClearPreservesSiblingAndFormerPointee(t *testing.T) {
	for _, registration := range []string{"", "defer "} {
		t.Run(registration+"clear", func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "clearstorage", `package clearstorage
type box struct{data *int}
type holder struct{values [1]*box;sibling *box}
func probe()(**box,**box,**int,*box,*int){
 data:=new(int);old:=&box{data:data};owner:=&holder{values:[1]*box{old},sibling:old}
 `+registration+`clear(owner.values[:])
 return &owner.values[0],&owner.sibling,&old.data,old,data
}`)
			fn := pkg.Func("probe")
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
				if returned.Block() == fn.Recover {
					continue
				}
				v := returned.Results
				if value, exact := ContentValue(v[0], returned); exact && DefinitelySame(value, v[3]) {
					t.Fatal("clear retained exact former element")
				}
				for _, pair := range [][2]int{{1, 3}, {2, 4}} {
					value, exact := ContentValue(v[pair[0]], returned)
					if !exact || !DefinitelySame(value, v[pair[1]]) {
						t.Fatalf("clear lost untouched storage %d: value=%v exact=%v", pair[0], value, exact)
					}
				}
				return
			}
			t.Fatal("missing normal observation")
		})
	}
}

func TestDeferredCopyUsesCapturedSlices(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "copycapture", `package copycapture
func probe()(**int,*int,*int){
 old:=new(int);replacement:=new(int);later:=new(int)
 dst:=[]*int{old};src:=[]*int{replacement};original:=dst
 defer copy(dst,src)
 dst=[]*int{later};src=[]*int{later};_=dst;_=src
 return &original[0],replacement,later
}`)
	fn := pkg.Func("probe")
	var dump strings.Builder
	if _, err := fn.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
		if returned.Block() == fn.Recover {
			continue
		}
		v := returned.Results
		for _, test := range []struct {
			value ssa.Value
			want  bool
		}{{v[1], true}, {v[2], false}} {
			contained, known := ContainsAt(v[0], test.value, returned)
			if !known || contained != test.want {
				t.Fatalf("captured copy containment=%v known=%v want %v", contained, known, test.want)
			}
		}
		return
	}
	t.Fatal("missing normal observation")
}

func TestBuiltinMutationInvalidatesCurrentAggregateButKeepsSnapshot(t *testing.T) {
	for _, operation := range []string{"clear(owner.values[:])", "copy(owner.values[:],[]*int{new(int)})"} {
		t.Run(operation, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "mutatedaggregate", `package mutatedaggregate
 type holder struct{values [1]*int}
 func probe()(**int,**int,*int){
 old:=new(int);source:=holder{values:[1]*int{old}};owner:=new(holder);*owner=source
 before:=*owner;`+operation+`;after:=*owner
 return &after.values[0],&before.values[0],old
 }`)
			fn := pkg.Func("probe")
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			returns := ssaflow.InstructionsOf[*ssa.Return](fn)
			if len(returns) != 1 {
				t.Fatalf("normal return count=%d, want one", len(returns))
			}
			returned := returns[0]
			v := returned.Results
			if value, exact := ContentValue(v[0], returned); exact && DefinitelySame(value, v[2]) {
				t.Fatal("builtin left a stale whole-aggregate view")
			}
			value, exact := ContentValue(v[1], returned)
			if !exact || !DefinitelySame(value, v[2]) {
				t.Fatal("builtin changed an earlier by-value snapshot")
			}
		})
	}
}
