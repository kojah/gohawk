package resourcelifetime

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// HEAD clients must be exact fresh zero-value allocations, stable local cells,
// or unchanged package defaults. Referrer and capture visits share the request
// allowance; shortened default-effect children retain explicit uncertainty.

func proveHeadClientUnconfiguredWithin(client ssa.Value, function *ssa.Function, budget *ssaflow.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if local, ok := client.(*ssa.Alloc); ok {
		return carriedValueProof(onlyHTTPDoUsesWithin(local, budget), resourceReasonUntouched, budget)
	}
	load, ok := client.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if ssaflow.ValueMatchesSymbol(load.X, httpDefaultClient) {
		if !onlyHTTPDoUsesWithin(load, budget) {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		return proveDefaultClientUnmodifiedWithin(function, budget)
	}
	cell, ok := load.X.(*ssa.Alloc)
	return carriedValueProof(ok && zeroClientCellWithin(cell, budget), resourceReasonUntouched, budget)
}

// zeroClientCell accepts a local variable that holds one fresh zero-value
// client and is only ever loaded for direct Do calls, here or inside a
// literal that captures it. A worker that shares the client can still
// change nothing about it. pmtiles keeps one client in a variable its
// download workers capture:
// https://github.com/protomaps/go-pmtiles/blob/a3e4951ea6a0477b784c27c1dcbfd9c130878c5a/pmtiles/sync.go#L73-L80
func zeroClientCellWithin(cell *ssa.Alloc, budget *ssaflow.SearchBudget) bool {
	if cell.Referrers() == nil {
		return false
	}
	stored := false
	for _, use := range *cell.Referrers() {
		if !budget.Spend() {
			return false
		}
		switch typed := use.(type) {
		case *ssa.DebugRef:
		case *ssa.Store:
			fresh, ok := typed.Val.(*ssa.Alloc)
			if stored || typed.Addr != cell || !ok || !onlyStoredIntoWithin(fresh, typed, budget) {
				return false
			}
			stored = true
		case *ssa.UnOp:
			if typed.Op != token.MUL || !onlyHTTPDoUsesWithin(typed, budget) {
				return false
			}
		case *ssa.MakeClosure:
			if !closureLoadsCellForDoWithin(typed, cell, budget) {
				return false
			}
		default:
			return false
		}
	}
	return stored
}

// onlyStoredInto reports whether the fresh allocation's only use is the store
// that puts it in the cell: no field was addressed, so it is zero-valued.
func onlyStoredIntoWithin(fresh *ssa.Alloc, store *ssa.Store, budget *ssaflow.SearchBudget) bool {
	if fresh.Referrers() == nil {
		return false
	}
	for _, use := range *fresh.Referrers() {
		if !budget.Spend() {
			return false
		}
		if _, debug := use.(*ssa.DebugRef); !debug && use != store {
			return false
		}
	}
	return true
}

func closureLoadsCellForDoWithin(closure *ssa.MakeClosure, cell *ssa.Alloc, budget *ssaflow.SearchBudget) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, pair := range ssaflow.ClosureBindingPairs(function, closure) {
		if !budget.Spend() {
			return false
		}
		if pair.Binding != cell {
			continue
		}
		free := pair.Free
		if free.Referrers() == nil {
			return false
		}
		for _, use := range *free.Referrers() {
			if !budget.Spend() {
				return false
			}
			load, loaded := use.(*ssa.UnOp)
			if _, debug := use.(*ssa.DebugRef); debug {
				continue
			}
			if !loaded || load.Op != token.MUL || !onlyHTTPDoUsesWithin(load, budget) {
				return false
			}
		}
	}
	return true
}

// Only direct Do calls may observe these fresh values. Field addresses, aliases
// and helper escapes would hide configuration or method mutation.
func onlyHTTPDoUsesWithin(value ssa.Value, budget *ssaflow.SearchBudget) bool {
	refs := value.Referrers()
	if refs == nil || len(*refs) == 0 {
		return false
	}
	for _, ref := range *refs {
		if !budget.Spend() {
			return false
		}
		call, ok := ref.(*ssa.Call)
		if !ok || !ssaflow.CallMatchesSymbol(call.Common(), httpClientDo) {
			return false
		}
	}
	return true
}
