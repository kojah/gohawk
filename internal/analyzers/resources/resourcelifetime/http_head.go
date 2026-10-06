package resourcelifetime

import (
	"go/token"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// HEAD clients must be exact fresh zero-value allocations, stable local cells,
// or unchanged package defaults. Referrer and capture visits share the request
// allowance; shortened default-effect children retain explicit uncertainty.

func proveHeadClientUnconfiguredWithin(client ssa.Value, function *ssa.Function, budget *proofs.SearchBudget) resourceProof {
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
	if ssacall.ValueMatchesSymbol(load.X, httpDefaultClient) {
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
func zeroClientCellWithin(cell *ssa.Alloc, budget *proofs.SearchBudget) bool {
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
func onlyStoredIntoWithin(fresh *ssa.Alloc, store *ssa.Store, budget *proofs.SearchBudget) bool {
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

func closureLoadsCellForDoWithin(closure *ssa.MakeClosure, cell *ssa.Alloc, budget *proofs.SearchBudget) bool {
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
func onlyHTTPDoUsesWithin(value ssa.Value, budget *proofs.SearchBudget) bool {
	refs := value.Referrers()
	if refs == nil || len(*refs) == 0 {
		return false
	}
	for _, ref := range *refs {
		if !budget.Spend() {
			return false
		}
		call, ok := ref.(*ssa.Call)
		if !ok || !ssacall.CallMatchesSymbol(call.Common(), httpClientDo) {
			return false
		}
	}
	return true
}

// HEAD provenance follows only exact constructor results and rebinding calls.
// Backward origin and forward immutable-use folds share mechanics and allowance;
// phis, aliases and unmodeled uses remain outside the contract.

// HEAD constructor and immutable-use queries share exact rebinding mechanics.
// Only call and extract roots are transparent; phis remain outside this policy.
func headConstructedWithin(request ssa.Value, budget *proofs.SearchBudget) bool {
	return (headRequestQuery{budget: budget}).prove(request)
}

type headRequestQuery struct {
	budget       *proofs.SearchBudget
	preserveUses bool
}

func headRequestWithin(request ssa.Value, budget *proofs.SearchBudget) bool {
	return (headRequestQuery{budget: budget, preserveUses: true}).prove(request)
}

func directHEADRequestValue(value ssa.Value) bool {
	switch value.(type) {
	case *ssa.Call, *ssa.Extract:
		return true
	}
	return false
}

// Preserve the former direct-chain boundary before entering the shared fold:
// phi fan-out could otherwise broaden a HEAD contract to merged requests.
func (query headRequestQuery) prove(request ssa.Value) bool {
	return directHEADRequestValue(request) && ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(query.budget).Every(request, query.origin)
}

func (query headRequestQuery) origin(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	switch typed := value.(type) {
	case *ssa.Extract:
		constructor, ok := typed.Tuple.(*ssa.Call)
		if !ok || typed.Index != 0 || !headConstructor(constructor.Common()) {
			return false
		}
	case *ssa.Call:
		if !ssacall.CallMatchesAnySymbol(typed.Common(), httpRequestWithContext, httpRequestClone) {
			return false
		}
		receiver := ssaflow.CallReceiver(typed.Common())
		if !directHEADRequestValue(receiver) || !walk.Every(receiver, query.origin) {
			return false
		}
	default:
		return false
	}
	return !query.preserveUses || ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(query.budget).Every(value, query.uses)
}

func headConstructor(common *ssa.CallCommon) bool {
	if ssacall.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "NewRequest")) {
		return len(common.Args) == 3 && constantString(common.Args[0]) == "HEAD"
	}
	if ssacall.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "NewRequestWithContext")) {
		return len(common.Args) == 4 && constantString(common.Args[1]) == "HEAD"
	}
	return false
}

// Origins and uses are separate folds. A backward chain visits the receiver,
// while this forward query must independently check every derived request for
// mutation or escape. Reusing the origin fold's visited set would discard valid
// rebinding evidence; sharing the allowance still bounds both directions.
func (query headRequestQuery) uses(walk ssaflow.ReachingWalk, request ssa.Value) bool {
	budget := query.budget
	refs := request.Referrers()
	if refs == nil || len(*refs) == 0 {
		return false
	}
	for _, ref := range *refs {
		if !budget.Spend() {
			return false
		}
		switch typed := ref.(type) {
		case *ssa.DebugRef:
		case *ssa.Call:
			common := typed.Common()
			if ssacall.CallMatchesSymbol(common, httpClientDo) && len(common.Args) == 2 && common.Args[1] == request {
				continue
			}
			if ssacall.CallMatchesAnySymbol(common, httpRequestWithContext, httpRequestClone) &&
				ssaflow.CallReceiver(common) == request && walk.Every(typed, query.uses) {
				continue
			}
			return false
		case *ssa.FieldAddr:
			if requestFieldName(typed) != "Header" || !headerUsesAreEditsWithin(typed, budget) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func requestFieldName(field *ssa.FieldAddr) string {
	structure := syntax.PointerStruct(field.X.Type())
	if structure == nil || !syntax.NamedType(field.X.Type(), "net/http", "Request") {
		return ""
	}
	return structure.Field(field.Field).Name()
}

// Only known header-map operations preserve the request contract. Sending the
// map to opaque code is not a general purity proof, even if Method is untouched
// by every use recognized so far; shortened use scans remain unavailable.
func headerUsesAreEditsWithin(field *ssa.FieldAddr, budget *proofs.SearchBudget) bool {
	if field.Referrers() == nil {
		return false
	}
	for _, use := range *field.Referrers() {
		if !budget.Spend() {
			return false
		}
		load, ok := use.(*ssa.UnOp)
		if !ok || load.Op != token.MUL || load.Referrers() == nil {
			return false
		}
		for _, edit := range *load.Referrers() {
			if !budget.Spend() {
				return false
			}
			call, ok := edit.(*ssa.Call)
			if !ok || ssaflow.CallReceiver(call.Common()) != load || !ssacall.CallMatchesAnySymbol(call.Common(), httpHeaderEdits...) {
				return false
			}
		}
	}
	return true
}

// Visible default-client and transport effects share one scan for HEAD and
// local header-only acquisitions. Only HEAD's root permits a default-client
// load used exclusively by Do; that allowance never enters helper summaries.
// Only visible bodies are expanded; recursive or shortened evidence cannot
// establish absence of visible changes. Hidden package effects stay opaque.

// httpEffectsBudget bounds visible-body effect queries. Both HTTP acquisition
// families retain this quota; exhaustion supplies the conservative may-modify
// answer rather than evidence that a global default is unchanged.
const httpEffectsBudget = 4000

// Default mutation keeps its existing child cap while sharing the caller's
// allowance. A shortened child is unavailable even when the caller can continue.
func proveDefaultClientUnmodifiedWithin(function *ssa.Function, budget *proofs.SearchBudget) resourceProof {
	child := budget.Within(httpEffectsBudget)
	modified := newHTTPWriterEffects().scanDefaultOverrides(function, child, true)
	return carriedValueProof(!modified, resourceReasonUntouched, child)
}

func (effects *httpWriterEffects) visibleOverrides(function *ssa.Function, budget *proofs.SearchBudget) bool {
	return effects.scanDefaultOverrides(function, budget, false)
}

func (effects *httpWriterEffects) scanDefaultOverrides(function *ssa.Function, budget *proofs.SearchBudget, allowRootDo bool) bool {
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		if allowRootDo {
			if load, ok := instruction.(*ssa.UnOp); ok && load.Op == token.MUL &&
				ssacall.ValueMatchesSymbol(load.X, httpDefaultClient) && onlyHTTPDoUsesWithin(load, budget) {
				continue
			}
		}
		for _, operand := range instruction.Operands(nil) {
			if !budget.Spend() {
				return true
			}
			if operand != nil && ssacall.ValueMatchesAnySymbol(*operand, httpDefaultClient, httpDefaultTransport) {
				return true
			}
		}
		callee, _ := ssacall.DirectCallee(ssaflow.InstructionCall(instruction))
		if callee != nil && len(callee.Blocks) != 0 && effects.overrides.Function(callee, budget) {
			return true
		}
	}
	return resourceFlowExhausted(budget)
}
