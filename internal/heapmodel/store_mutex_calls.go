package heapmodel

import (
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Standard mutex operations mutate their receiver's internal state; they do
// not retain a pointer to the enclosing user object after a synchronous call.
// Forget only that storage, preserving sibling fields and their destinations.
// A launched call still exposes the receiver to another goroutine, and an
// interface call or RLocker adapter does not satisfy this direct-call contract.
// https://go.dev/src/sync/mutex.go
// https://go.dev/src/sync/rwmutex.go
var synchronousMutexMethods = []syntax.Symbol{
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Lock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Unlock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "TryLock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Lock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Unlock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "TryLock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RLock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RUnlock"}),
	syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "TryRLock"}),
}

func (graph *regionGraph) definedMutexCall(state *regionState, common *ssa.CallCommon, instruction ssa.Instruction, started bool) bool {
	if started || common.StaticCallee() == nil || len(common.Args) != 1 || !ssacall.CallMatchesAnySymbol(common, synchronousMutexMethods...) {
		return false
	}
	receiver := graph.pointees(common.Args[0])
	if _, known := singleSlot(receiver); !known {
		return false
	}
	// Reuse exact slot invalidation from summary substitution. Whole-object
	// clobber would lose sibling map/slice identities and hide borrowed storage.
	substitution := heapSubstitution{graph: graph, state: state, instruction: instruction}
	substitution.forgetSlots(receiver)
	return true
}
