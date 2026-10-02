package resourcelifetime

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// HEAD provenance follows only exact constructor results and rebinding calls.
// Backward origin and forward immutable-use folds share mechanics and allowance;
// phis, aliases and unmodeled uses remain outside the contract.

// HEAD constructor and immutable-use queries share exact rebinding mechanics.
// Only call and extract roots are transparent; phis remain outside this policy.
func headConstructedWithin(request ssa.Value, budget *ssaflow.SearchBudget) bool {
	return (headRequestQuery{budget: budget}).prove(request)
}

type headRequestQuery struct {
	budget       *ssaflow.SearchBudget
	preserveUses bool
}

func headRequestWithin(request ssa.Value, budget *ssaflow.SearchBudget) bool {
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
		if !ssaflow.CallMatchesAnySymbol(typed.Common(), httpRequestWithContext, httpRequestClone) {
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
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "NewRequest")) {
		return len(common.Args) == 3 && constantString(common.Args[0]) == "HEAD"
	}
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "NewRequestWithContext")) {
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
			if ssaflow.CallMatchesSymbol(common, httpClientDo) && len(common.Args) == 2 && common.Args[1] == request {
				continue
			}
			if ssaflow.CallMatchesAnySymbol(common, httpRequestWithContext, httpRequestClone) &&
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
func headerUsesAreEditsWithin(field *ssa.FieldAddr, budget *ssaflow.SearchBudget) bool {
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
			if !ok || ssaflow.CallReceiver(call.Common()) != load || !ssaflow.CallMatchesAnySymbol(call.Common(), httpHeaderEdits...) {
				return false
			}
		}
	}
	return true
}
