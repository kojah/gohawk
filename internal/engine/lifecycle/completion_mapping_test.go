package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// A receiver captured by a closure is spilled to a cell. When the cell is
// written once, the lock's owner and a later argument are two loads of one
// value, and the lock is storage beneath that argument.
func TestSameValueStorageOwner(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerprobe", `package ownerprobe
import "sync"
type T struct{ mu sync.Mutex; n int }
type N struct{ child T }
func use(*T) {}
func nested(s *N) { s.child.mu.Lock(); use(&s.child) }
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
	for name, want := range map[string]bool{"once": true, "twice": false, "nested": true} {
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
		checkStorageOwnerAllowance(t, target, argument, want)
	}
}

// Deferred cleanup of embedded storage requires an unchanged owner cell and
// the exact field path. Pointee writes do not replace the owner pointer.
func TestDeferredCapturedStorageOwner(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerprobe", `package ownerprobe
import "sync"
type T struct { mu, other sync.Mutex; n int }
func exact(s *T) {
 s.mu.Lock()
 defer func() { s.n = 0; s.mu.Unlock() }()
}
func sibling(s *T) {
 s.mu.Lock()
 defer func() { s.other.Unlock() }()
}
func replacedAfter(s, other *T) {
 s.mu.Lock()
 defer func() { s.mu.Unlock() }()
 s = other
}
func replacedBefore(s, other *T) {
 s.mu.Lock()
 s = other
 defer func() { s.mu.Unlock() }()
}
func changedByCleanup(s, other *T) {
 s.mu.Lock()
 defer func() { s = other; s.mu.Unlock() }()
}
func expose(**T)
func opaqueCell(s *T) {
 s.mu.Lock()
 defer func() { s.mu.Unlock() }()
 expose(&s)
}
`)
	for name, want := range map[string]bool{
		"exact": true, "sibling": false, "replacedAfter": false,
		"replacedBefore": false, "changedByCleanup": false, "opaqueCell": false,
	} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var target ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
				if ssaflow.CallName(call.Common()) == "Lock" {
					target = ssaflow.CallReceiver(call.Common())
				}
			}
			deferred := ssaflow.InstructionsOf[*ssa.Defer](function)[0]
			proof := ProveCompletion(CompletionRequest{
				Instruction: deferred, Target: target, Methods: []string{"Unlock"},
				Budget: proofs.NewSearchBudget(proofs.QueryBudget),
			})
			if proof.Proven() != want {
				t.Errorf("deferred embedded cleanup = %+v, want proven %t", proof, want)
			}
		})
	}
}

func checkStorageOwnerAllowance(t *testing.T, target, argument ssa.Value, want bool) {
	t.Helper()
	if got := sameValueStorageOwner(target, argument, nil) != nil; got != want {
		t.Fatalf("default owner=%v, want %v", got, want)
	}
	for limit := 1; limit <= proofs.QueryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		owner := sameValueStorageOwner(target, argument, budget)
		if budget.Exhausted() {
			if owner != nil {
				t.Fatalf("cut %d publishes owner %v", limit, owner)
			}
			continue
		}
		if (owner != nil) != want {
			t.Fatalf("complete %d: owner=%v, want found %v", limit, owner, want)
		}
		return
	}
	t.Fatal("owner query never completed")
}
