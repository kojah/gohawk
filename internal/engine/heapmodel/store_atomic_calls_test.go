package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAtomicCallKeepsConditionalContents(t *testing.T) {
	for _, test := range []struct {
		name, typ, call string
		old             bool
	}{
		{"pointerStore", "atomic.Pointer[int]", "cell.Store(replacement)", false},
		{"pointerSwap", "atomic.Pointer[int]", "cell.Swap(replacement)", false},
		{"pointerCAS", "atomic.Pointer[int]", "cell.CompareAndSwap(nil,replacement)", true},
		{"valueStore", "atomic.Value", "cell.Store(replacement)", false},
		{"valueSwap", "atomic.Value", "cell.Swap(replacement)", false},
		{"valueCAS", "atomic.Value", "cell.CompareAndSwap(nil,replacement)", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "atomiccontents", `package atomiccontents
 import "sync/atomic"
 func inspect(any,*int,*int){}
 func probe(){old:=new(int);replacement:=new(int);cell:=new(`+test.typ+`);cell.Store(old);`+test.call+`;inspect(cell,old,replacement)}
 `)
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("probe")) {
				if ssaflow.CallName(call.Common()) != "inspect" {
					continue
				}
				args := call.Common().Args
				checkAtomicContents(t, args[0], args[1], args[2], call, test.old)
				return
			}
			t.Fatal("missing compiled observation")
		})
	}
}

func checkAtomicContents(t *testing.T, cell, old, replacement ssa.Value, at ssa.Instruction, keepOld bool) {
	t.Helper()
	retained, known := ContainsAt(cell, old, at)
	if !known || retained != keepOld {
		t.Fatalf("old retained=%v known=%v want %v", retained, known, keepOld)
	}
	retained, known = ContainsAt(cell, replacement, at)
	if !known || !retained {
		t.Fatalf("replacement retained=%v known=%v", retained, known)
	}
	contents, exact := ContentValue(cell, at)
	if keepOld && exact {
		t.Fatalf("conditional update published exact contents %v", contents)
	}
	if !keepOld && (!exact || !DefinitelySame(contents, replacement)) {
		t.Fatalf("unconditional contents=%v exact=%v", contents, exact)
	}
}

func TestDeferredAtomicContents(t *testing.T) {
	for _, test := range []struct {
		name, typ, operation string
		old                  bool
	}{
		{"pointerStore", "atomic.Pointer[int]", "Store(replacement)", false},
		{"pointerSwap", "atomic.Pointer[int]", "Swap(replacement)", false},
		{"pointerCAS", "atomic.Pointer[int]", "CompareAndSwap(nil,replacement)", true},
		{"valueStore", "atomic.Value", "Store(replacement)", false},
		{"valueSwap", "atomic.Value", "Swap(replacement)", false},
		{"valueCAS", "atomic.Value", "CompareAndSwap(nil,replacement)", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "deferredatomic", `package deferredatomic
 import "sync/atomic"
 func probe()(any,*int,*int){
 old:=new(int);replacement:=new(int);cell:=new(`+test.typ+`)
 cell.Store(old);defer cell.`+test.operation+`;return cell,old,replacement
 }
 `)
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](pkg.Func("probe")) {
				// Named result loads can follow RunDefers. The separate recovery
				// return has no RunDefers and is not this observation.
				normal := false
				for _, instruction := range returned.Block().Instrs {
					if _, ok := instruction.(*ssa.RunDefers); ok {
						normal = true
					}
				}
				if !normal {
					continue
				}
				values := returned.Results
				checkAtomicContents(t, values[0], values[1], values[2], returned, test.old)
				return
			}
			var dump strings.Builder
			if _, err := pkg.Func("probe").WriteTo(&dump); err != nil {
				t.Fatalf("dump compiled function: %v", err)
			}
			t.Fatalf("missing post-defer observation:\n%s", dump.String())
		})
	}
}
