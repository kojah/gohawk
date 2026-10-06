package heapmodel

import (
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Atomic cells retain values through stores; they do not make the pointee's
// identity opaque just because the implementation uses unsafe.Pointer.
// Store and Swap replace contents. CompareAndSwap may leave the old value,
// so its possible update must never publish a must-store or success guarantee.
// https://go.dev/src/sync/atomic/type.go
// https://go.dev/src/sync/atomic/value.go
var (
	atomicStores = []syntax.Symbol{
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync/atomic", Receiver: "Pointer", Name: "Store"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync/atomic", Receiver: "Pointer", Name: "Swap"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync/atomic", Receiver: "Value", Name: "Store"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync/atomic", Receiver: "Value", Name: "Swap"}),
	}
	atomicCompareAndSwaps = []syntax.Symbol{
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync/atomic", Receiver: "Pointer", Name: "CompareAndSwap"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync/atomic", Receiver: "Value", Name: "CompareAndSwap"}),
	}
)

type atomicWrite struct {
	cell, value ssa.Value
	conditional bool
}

func (graph *regionGraph) definedAtomicCall(state *regionState, common *ssa.CallCommon, instruction ssa.Instruction, started bool) bool {
	write, known := atomicStore(common)
	if !known || started {
		return false
	}
	if write.conditional {
		graph.storeConditionallyInto(state, write.cell, write.value, instruction)
	} else {
		graph.storeInto(state, write.cell, write.value, instruction)
	}
	return true
}

func atomicStore(common *ssa.CallCommon) (atomicWrite, bool) {
	switch {
	case ssacall.CallMatchesAnySymbol(common, atomicStores...) && len(common.Args) == 2:
		return atomicWrite{cell: common.Args[0], value: common.Args[1]}, true
	case ssacall.CallMatchesAnySymbol(common, atomicCompareAndSwaps...) && len(common.Args) == 3:
		return atomicWrite{cell: common.Args[0], value: common.Args[2], conditional: true}, true
	default:
		return atomicWrite{}, false
	}
}
