package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
