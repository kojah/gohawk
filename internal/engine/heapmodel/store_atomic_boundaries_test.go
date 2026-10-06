package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredAtomicCapturesRegistrationValues(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "atomiccapture", `package atomiccapture
import "sync/atomic"
func probe()(any,*int,*int){
 old:=new(int);replacement:=new(int);cell:=new(atomic.Pointer[int]);cell.Store(old)
 original:=cell;captured:=replacement
 defer cell.Store(replacement)
 cell=new(atomic.Pointer[int]);replacement=new(int)
 cell.Store(replacement)
 return original,old,captured
}`)
	fn := pkg.Func("probe")
	for _, run := range ssaflow.InstructionsOf[*ssa.RunDefers](fn) {
		instructions := run.Block().Instrs
		returned, ok := instructions[len(instructions)-1].(*ssa.Return)
		if !ok {
			continue
		}
		values := returned.Results
		checkAtomicContents(t, values[0], values[1], values[2], returned, false)
		return
	}
	t.Fatal("missing compiled post-defer return")
}

func TestAtomicOpaqueEffectsDoNotEstablishContents(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"async", `go cell.Store(replacement)`},
		{"conditionalDefer", `if flag{defer cell.Store(replacement)}`},
		{"repeatedDefer", `for i:=0;i<2;i++{defer cell.Store(replacement)}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "atomicopaque", `package atomicopaque
import "sync/atomic"
func probe(flag bool)(*atomic.Pointer[int],*int){
 cell:=new(atomic.Pointer[int]);replacement:=new(int);`+test.body+`
 return cell,replacement
}`)
			fn := pkg.Func("probe")
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
				if returned.Block() == fn.Recover {
					continue
				}
				cell := returned.Results[0]
				if contents, exact := ContentValue(cell, returned); exact {
					t.Fatalf("opaque update established contents %v", contents)
				}
				if object, known := ExclusiveAt(cell, returned); known && object.Local {
					t.Fatal("opaque update retained exclusive storage")
				}
				return
			}
			t.Fatal("missing compiled normal return")
		})
	}
}

func TestAtomicMethodLookalikeDoesNotReplaceContents(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "atomiclookalike", `package atomiclookalike
type fake struct{value *int}
func(c *fake)CompareAndSwap(old,replacement *int)bool{return false}
func inspect(*fake,*int){}
func probe(){old:=new(int);cell:=&fake{value:old};cell.CompareAndSwap(nil,new(int));inspect(cell,old)}
`)
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("probe")) {
		if ssaflow.CallName(call.Common()) != "CompareAndSwap" {
			continue
		}
		if _, known := atomicStore(call.Common()); known {
			t.Fatal("project method received standard atomic contract")
		}
		return
	}
	t.Fatal("missing compiled project method call")
}

func TestAtomicHeapPublicationKeepsConditionalEdges(t *testing.T) {
	for _, typ := range []string{"atomic.Pointer[int]", "atomic.Value"} {
		for _, deferred := range []string{"", "defer "} {
			for _, operation := range []string{"Store(replacement)", "Swap(replacement)", "CompareAndSwap(nil,replacement)"} {
				t.Run(typ+"/"+deferred+operation, func(t *testing.T) {
					pkg := ssaflowtest.BuildPackage(t, "atomicpublication", `package atomicpublication
import "sync/atomic"
func probe(cell *`+typ+`,old,replacement *int){cell.Store(old);`+deferred+`cell.`+operation+`}
`)
					summary, known := ProjectHeap(pkg.Func("probe"))
					if !known {
						t.Fatal("heap publication unavailable")
					}
					conditional := operation == "CompareAndSwap(nil,replacement)"
					found := false
					for _, edge := range summary.Edges {
						if edge.From.Root.Kind != HeapParameter || edge.From.Root.Index != 0 || edge.From.Path != "" ||
							edge.To.Kind != HeapTargetSlot || edge.To.Slot.Root.Kind != HeapParameter || edge.To.Slot.Root.Index != 2 {
							continue
						}
						found = true
						if edge.Must == conditional {
							t.Fatalf("replacement must=%v conditional=%v:\n%s", edge.Must, conditional, summary.String())
						}
					}
					if !found {
						t.Fatalf("missing replacement publication:\n%s", summary.String())
					}
				})
			}
		}
	}
}
