package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
