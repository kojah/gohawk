package heapmodel

import (
	"strings"
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

func TestMutexCallsPreserveLocalDestination(t *testing.T) {
	for _, test := range []struct {
		name, receiver, body string
		local                bool
	}{
		{"read", "sync.RWMutex", "o.mu.RLock()", true},
		{"write", "sync.RWMutex", "o.mu.Lock()", true},
		{"tryRead", "sync.RWMutex", "o.mu.TryRLock()", true},
		{"tryWrite", "sync.RWMutex", "o.mu.TryLock()", true},
		{"readRelease", "sync.RWMutex", "o.mu.RLock();o.mu.RUnlock()", true},
		{"writeRelease", "sync.RWMutex", "o.mu.Lock();o.mu.Unlock()", true},
		{"mutexWrite", "sync.Mutex", "o.mu.Lock()", true},
		{"mutexTry", "sync.Mutex", "o.mu.TryLock()", true},
		{"mutexRelease", "sync.Mutex", "o.mu.Lock();o.mu.Unlock()", true},
		{"async", "sync.RWMutex", "go o.mu.RLock()", false},
		{"adapter", "sync.RWMutex", "o.mu.RLocker().Lock()", false},
		{"lookalike", "mutex", "o.mu.RLock()", false},
		{"published", "sync.RWMutex", "global=o;o.mu.RLock()", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `package mutexheap
 import "sync"
 var _ sync.Mutex
 type holder struct{mu ` + test.receiver + `;count int}
 var global *holder
 type mutex struct{value int}
 var retained *mutex
 func(m *mutex)RLock(){retained=m}
 func subject(){o:=new(holder);` + test.body + `;o.count=1}
 `
			pkg := ssaflowtest.BuildPackage(t, "mutexheap", source)
			fn := pkg.Func("subject")
			for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
				field, ok := store.Addr.(*ssa.FieldAddr)
				if !ok || field.Field != 1 {
					continue
				}
				object, known := ExclusiveAt(field, store)
				if local := known && object.Local; local != test.local {
					t.Fatalf("local=%v known=%v object=%+v", local, known, object)
				}
				return
			}
			t.Fatal("missing compiled mutation")
		})
	}
}

func TestCallApplicationReasonCodes(t *testing.T) {
	want := map[CallApplicationReason]string{
		CallApplicationUnknown: "unknown", CallSummaryApplied: "summary-applied",
		CallNoSummary: "no-summary", CallClosure: "closure-callee", CallInterface: "interface-call",
		CallDynamic: "dynamic-call", CallStarted: "started", CallRecursive: "call-cycle",
		CallDeferredUncertain: "defer-registration-uncertain",
	}
	if len(want) != int(callApplicationReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range callApplicationReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []CallApplicationReason{callApplicationReasonCount, 255} {
		if reason.String() != "invalid-call-application-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

func TestDeferredMutexExecutionKeepsPrivateOwner(t *testing.T) {
	for _, test := range []struct {
		name, typ, body string
		local           bool
	}{
		{"readUnlock", "sync.RWMutex", `o.mu.RLock();defer o.mu.RUnlock()`, true},
		{"unlock", "sync.Mutex", `o.mu.Lock();defer o.mu.Unlock()`, true},
		{"conditional", "sync.RWMutex", `o.mu.RLock();if flag{defer o.mu.RUnlock()}`, false},
		{"repeated", "sync.RWMutex", `for i:=0;i<2;i++{o.mu.RLock();defer o.mu.RUnlock()}`, false},
		{"lookalike", "fake", `defer o.mu.RUnlock()`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "deferredmutex", `package deferredmutex
 import "sync"
 var _ sync.Mutex
 type fake struct{}
 var retained *fake
 func(m *fake)RUnlock(){retained=m}
 type holder struct{mu `+test.typ+`;count int}
 func probe(flag bool){o:=new(holder);`+test.body+`}
 `)
			fn := pkg.Func("probe")
			owner := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
			for _, run := range ssaflow.InstructionsOf[*ssa.RunDefers](fn) {
				block := run.Block()
				returned := block.Instrs[len(block.Instrs)-1]
				if _, ok := returned.(*ssa.Return); !ok {
					continue
				}
				object, known := ExclusiveAt(owner, returned)
				if local := known && object.Local; local != test.local {
					t.Fatalf("local=%v known=%v object=%+v", local, known, object)
				}
				return
			}
			t.Fatal("missing compiled post-defer return")
		})
	}
}
