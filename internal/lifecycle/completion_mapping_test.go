package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// A receiver captured by a closure is spilled to a cell. When the cell is
// written once, the lock's owner and a later argument are two loads of one
// value, and the lock is storage beneath that argument.
func TestSameValueStorageOwner(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerprobe", `package ownerprobe
import "sync"
type T struct{ mu sync.Mutex; n int }
func use(*T) {}
func once(s *T) {
	keep := func() { s.n++ }
	_ = keep
	s.mu.Lock()
	use(s)
}
func twice(s, other *T) {
	keep := func() { s.n++ }
	_ = keep
	s.mu.Lock()
	s = other
	use(s)
}
`)
	for name, want := range map[string]bool{"once": true, "twice": false} {
		var target, argument ssa.Value
		for _, block := range pkg.Func(name).Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				switch ssaflow.CallName(call.Common()) {
				case "Lock":
					target = ssaflow.CallReceiver(call.Common())
				case "use":
					argument = call.Common().Args[0]
				}
			}
		}
		if target == nil || argument == nil {
			t.Fatalf("%s: lock target or argument not found", name)
		}
		if got := sameValueStorageOwner(target, argument) != nil; got != want {
			t.Errorf("%s: owner found = %t, want %t", name, got, want)
		}
	}
}
