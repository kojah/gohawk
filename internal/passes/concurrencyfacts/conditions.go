package concurrencyfacts

// Conditions retain their own identity in summaries. The associated locker
// can only be recovered from an exact NewCond call, never a mutable L field.
import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

var (
	newCond  = syntax.PackageFunction("sync", "NewCond")
	condWait = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Cond", Name: "Wait"})
)

func condPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "sync", "Cond")
}

func condLocker(call ssa.CallInstruction) (ssa.Value, bool) {
	if !ssaflow.CallMatchesSymbol(call.Common(), newCond) || len(call.Common().Args) != 1 {
		return nil, false
	}
	wrapped, ok := call.Common().Args[0].(*ssa.MakeInterface)
	if !ok || !MutexPointer(wrapped.X.Type()) {
		return nil, false
	}
	return wrapped.X, true
}

// CondMutex resolves the exact Mutex supplied to a NewCond allocation. It does
// not prove that L remains unchanged: callers need a complete root summary,
// which rejects condition-field access, mutation and opaque publication.
func (engine *Engine) CondMutex(reference Reference, budget *ssaflow.SearchBudget) (Reference, bool) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	call, ok := reference.Value.(*ssa.Call)
	if !ok || reference.Indirect {
		return Reference{}, false
	}
	locker, ok := condLocker(call)
	if !ok {
		return Reference{}, false
	}
	return engine.query(budget).reference(locker)
}
