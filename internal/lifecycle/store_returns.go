package lifecycle

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// ReturnedValueOwnsValue reports whether any returned value carries value,
// directly or inside an aggregate or callback.
func ReturnedValueOwnsValue(returned *ssa.Return, value ssa.Value) bool {
	return ProveReturnedOwnershipWithin(returned, value, nil, nil).Proven()
}

// ReturnsOwner reports whether callee returns a value that owns its parameter
// at index. A callee in another package has no body to read here, so the
// answer comes from its summary, which lives above this package.
type ReturnsOwner func(callee *ssa.Function, index int) bool

// ReturnedValueOwnsValueSummarized is ReturnedValueOwnsValue for a caller that
// can answer for a callee whose body is unavailable. A wrapping constructor
// commonly delegates across a package boundary, as encoding/csv reaches its
// reader through bufio and encoding/json reaches its through jsontext, and
// without the summary the search stops at that boundary and concludes the
// callee kept the argument for itself.
func ReturnedValueOwnsValueSummarized(returned *ssa.Return, value ssa.Value, summarized ReturnsOwner) bool {
	return ProveReturnedOwnershipWithin(returned, value, summarized, nil).Proven()
}

// ProveReturnedOwnershipWithin asks the existing possible-ownership search with
// caller allowance. Result, value, reference, storage and constructor coverage
// visits share budget. Cutoff is unknown; a completed negative means only that
// this model found no owner. Graph/type and summary-hook internals remain separate.
func ProveReturnedOwnershipWithin(returned *ssa.Return, value ssa.Value, summarized ReturnsOwner, budget *proofs.SearchBudget) proofs.Proof {
	search := newOwnershipSearch(summarized)
	search.budget = budget
	found := search.returnedValueOwnsValue(returned, value)
	if search.exhausted() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	state, reason := proofs.EvidenceDisproven, proofs.EvidenceNotFound
	if found {
		state, reason = proofs.EvidenceProven, proofs.EvidenceStructuralWalk
	}
	return proofs.Proof{State: state, Reason: reason, Provenance: proofs.EvidenceFromLocalSSA}
}

// ownershipSearch carries the cycle guard and the summary hook through the
// walk that asks whether a returned value holds another value.
type ownershipSearch struct {
	seen       map[ownershipPair]bool
	summarized ReturnsOwner
	budget     *proofs.SearchBudget
}

func newOwnershipSearch(summarized ReturnsOwner) *ownershipSearch {
	return &ownershipSearch{seen: map[ownershipPair]bool{}, summarized: summarized}
}

func (search *ownershipSearch) exhausted() bool {
	return search.budget.Exhausted() || search.budget.PoolExhausted()
}

func (search *ownershipSearch) aliases(left, right ssa.Value) bool {
	return search.budget.Spend() && heapmodel.MayAlias(left, right)
}

type ownershipPair struct {
	aggregate ssa.Value
	value     ssa.Value
}

func (search *ownershipSearch) returnedValueOwnsValue(returned *ssa.Return, value ssa.Value) bool {
	if returned == nil {
		return false
	}
	for _, result := range returned.Results {
		if !search.budget.Spend() {
			return false
		}
		if search.aggregateStoresValue(result, value) {
			return true
		}
	}
	return false
}

func (search *ownershipSearch) aggregateStoresValue(aggregate, value ssa.Value) bool {
	if !search.budget.Spend() {
		return false
	}
	pair := ownershipPair{aggregate: aggregate, value: value}
	if aggregate == nil || search.seen[pair] {
		return false
	}
	// Alias evidence belongs at this entry, so returns, loads, stores and
	// constructor arguments ask the same question once. Only a negative alias
	// answer enters the cycle guard; a direct alias remains independently usable
	// on later visits. Containment below remains possible ownership, not release.
	if search.aliases(aggregate, value) {
		return true
	}
	search.seen[pair] = true
	// A constructor with a fast path returns the argument itself once it is
	// already the type it would wrap, as bufio.NewReaderSize does with
	// rd.(*Reader). The assertion selects the same object, so the caller
	// receives back what it passed in and still owns it.
	if inner, ok := ssaflow.UnwrapTransparentValue(
		aggregate, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|
			ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface|ssaflow.TransparentTypeAssert,
	); ok {
		return search.aggregateStoresValue(inner, value)
	}
	switch typed := aggregate.(type) {
	case *ssa.Call:
		if search.callAggregateStoresValue(typed, value) {
			return true
		}
	case *ssa.Phi:
		for _, incoming := range ssaflow.PhiIncoming(typed) {
			if !search.budget.Spend() {
				return false
			}
			if search.aggregateStoresValue(incoming, value) {
				return true
			}
		}
	case *ssa.Slice:
		// Stores can select an element through this slice rather than its
		// backing array. Inspect those uses too when the backing owner does not
		// account for the value; this remains possible containment, not release.
		// https://github.com/criyle/go-sandbox/blob/6a60e40be9d0cefb656c4ae12415c5fd040df954/cmd/runprog/fileutil.go#L6-L32
		if search.aggregateStoresValue(typed.X, value) {
			return true
		}
	case *ssa.MakeClosure:
		// A returned callback that captured the value keeps it alive and is the
		// only thing that can still release it, so the caller receives the
		// obligation with the callback.
		if closureBindingsOwnValueWithin(typed, value, search.budget, func(binding ssa.Value) bool {
			return search.aggregateStoresValue(binding, value)
		}) {
			return true
		}
	case *ssa.UnOp:
		if search.loadStoresValue(typed, value) {
			return true
		}
	}
	if _, ok := aggregate.(*ssa.Alloc); ok && search.addressStoresValue(aggregate, value) {
		return true
	}
	return search.aggregateReferrersStoreValue(aggregate, value)
}

// loadStoresValue decides the load case of aggregateStoresValue.
func (search *ownershipSearch) loadStoresValue(typed *ssa.UnOp, value ssa.Value) bool {
	// Copying the owner by value, as in ephemeral(*cmd), carries the same
	// process or handle state, so returning the copy transfers it. A struct
	// literal returned by value is likewise loaded from the local that
	// assembled it, so the load carries whatever that local's fields hold.
	if typed.Op == token.MUL && search.aggregateStoresValue(typed.X, value) {
		return true
	}
	// A load of one element or field of a local aggregate may carry what
	// any store to that same path put there: wireguard-go opens files
	// into an array on either branch and then lists the elements in
	// os.ProcAttr.Files for the child process. Each element expression
	// is its own address instruction, so the stores are found by the
	// path beneath the root rather than by the load's own address. This
	// is possible containment, which suits a boundary and never a proof.
	// https://github.com/WireGuard/wireguard-go/blob/66f4d31457f4be14e4b4a72c5ec6a4a0fa2ba9b9/main.go#L183-L200
	if typed.Op == token.MUL && search.samePathStoresValue(typed.X, value) {
		return true
	}
	return false
}

func (search *ownershipSearch) anyAggregateStoresValue(aggregates []ssa.Value, value ssa.Value) bool {
	for _, aggregate := range aggregates {
		if !search.budget.Spend() {
			return false
		}
		if search.aggregateStoresValue(aggregate, value) {
			return true
		}
	}
	return false
}

func (search *ownershipSearch) aggregateReferrersStoreValue(aggregate, value ssa.Value) bool {
	if aggregate.Referrers() == nil {
		return false
	}
	for _, reference := range *aggregate.Referrers() {
		if !search.budget.Spend() {
			return false
		}
		if call, ok := reference.(ssa.CallInstruction); ok {
			if search.callStoresValueIntoAggregate(call, aggregate, value) {
				return true
			}
			continue
		}
		address, ok := reference.(ssa.Value)
		if !ok {
			continue
		}
		switch address.(type) {
		case *ssa.FieldAddr, *ssa.IndexAddr:
			if search.addressStoresValue(address, value) || search.aggregateStoresValue(address, value) {
				return true
			}
		}
	}
	return false
}

// samePathStoresValue reports whether some store to the same field or
// element path beneath the same local root as address stores the value.
func (search *ownershipSearch) samePathStoresValue(address ssa.Value, value ssa.Value) bool {
	root := localAggregateRootWithin(address, search.budget)
	if root == nil {
		return false
	}
	path, ok := ssaflow.AccessPathStepsWithin(address, root, search.budget)
	if !ok || len(path) == 0 {
		return false
	}
	for _, selection := range heapmodel.SelectionsOfWithin(root, path, search.budget) {
		if search.addressStoresValue(selection, value) {
			return true
		}
	}
	return false
}

// localAggregateRoot returns the local allocation an address selects
// beneath through fields and constant indexes, or nil.
func localAggregateRootWithin(address ssa.Value, budget *proofs.SearchBudget) *ssa.Alloc {
	for {
		if !budget.Spend() {
			return nil
		}
		switch typed := address.(type) {
		case *ssa.Alloc:
			return typed
		case *ssa.FieldAddr:
			address = typed.X
		case *ssa.IndexAddr:
			address = typed.X
		default:
			return nil
		}
	}
}

func (search *ownershipSearch) addressStoresValue(address ssa.Value, value ssa.Value) bool {
	for stored := range StoredIntoWithin(address, search.budget) {
		if search.aggregateStoresValue(stored, value) {
			return true
		}
	}
	return false
}

// LoadedAggregateMayHold reports whether value is a load of a whole aggregate,
// seen through transparent wrappers and phi merges, out of storage that had a
// value which may alias target stored into it, directly or beneath a field or
// element. This is containment, not identity: the loaded copy carries the
// target without being it, which is why MayAlias stops at the copy. A caller
// that must count a stored or returned copy as a store or return of what it
// holds, as the retention summary does for bufio.NewReader's reset writing a
// composite literal over the receiver, asks this question alongside MayAlias.
func LoadedAggregateMayHold(value, target ssa.Value) bool {
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	return target != nil && ssaflow.NewReachingWalk(forms).Any(value, func(_ ssaflow.ReachingWalk, value ssa.Value) bool {
		load, ok := value.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			return false
		}
		for stored := range StoredInto(load.X) {
			if heapmodel.MayAlias(stored, target) {
				return true
			}
		}
		return false
	})
}

// ReturnedResultWithin resolves the value handed back at index under budget.
// Deferred results are loaded from cells; observation-time storage determines
// the value at that load. A nil budget preserves the default query policy.
// Unavailable storage retains the original load; cutoff returns nil and leaves
// exhaustion visible on budget, so it cannot impersonate a nil error or success.
func ReturnedResultWithin(returned *ssa.Return, index int, budget *proofs.SearchBudget) ssa.Value { //nolint:ireturn // Preserve SSA result identity.
	if index < 0 || index >= len(returned.Results) {
		return nil
	}
	if !budget.Spend() {
		return nil
	}
	result := returned.Results[index]
	if load, ok := result.(*ssa.UnOp); ok && load.Op == token.MUL {
		stored := heapmodel.NewStorage(budget).Content(load.X, load)
		if budget.Exhausted() || budget.PoolExhausted() {
			return nil
		}
		if stored.Proven() {
			return stored.Value
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil
	}
	return result
}

// ReturnsParameterUnchanged reports whether every normal return of function
// hands back parameter itself, under the same static type, at result index.
// Identity is exact storage identity, not derivation: a wrapper, a
// conversion to an interface, or a value chosen between the parameter and
// something else is not the parameter. A body with no reachable normal
// return proves nothing.
func ReturnsParameterUnchanged(function *ssa.Function, parameter ssa.Value, index int) bool {
	return ProveReturnedParameterWithin(function, parameter, index, nil).Proven()
}

// ProveReturnedParameterWithin requires a reachable normal return and exact
// same-type parameter identity at every return. Reachability, coverage and
// storage comparisons share budget. Cutoff and unresolved identity are unknown,
// never evidence that no counterexample exists. A nil budget retains default
// flow policy and the storage engine's own allowance; graph construction and
// type-system internals have independent costs.
func ProveReturnedParameterWithin(function *ssa.Function, parameter ssa.Value, index int, budget *proofs.SearchBudget) proofs.Proof {
	unknown := proofs.Proof{Reason: proofs.EvidenceUnavailable}
	if function == nil || len(function.Blocks) == 0 || parameter == nil || index < 0 {
		return unknown
	}
	reachable := ssapath.ProveNormalReturnWithin(function.Blocks[0], nil, budget)
	if !reachable.Proven() {
		unknown.Reason = reachable.Reason
		return unknown
	}
	storage := heapmodel.NewStorage(budget)
	outcome := ssapath.EvaluateObligationFromEntry(function, ssapath.ObligationFlow{
		Budget: budget, Instruction: ssapath.ExactOrNone(nil),
		Return: func(returned *ssa.Return) ssapath.ObligationAction {
			if !budget.Spend() || index >= len(returned.Results) {
				return ssapath.ObligationUnknown
			}
			result := returned.Results[index]
			if types.Identical(result.Type(), parameter.Type()) && storage.Same(result, parameter).Proven() {
				return ssapath.ObligationExact
			}
			return ssapath.ObligationUnknown
		},
	})
	if budget.Exhausted() || storage.Budget().Exhausted() {
		unknown.Reason = proofs.EvidenceBudgetExhausted
		return unknown
	}
	if outcome != ssapath.ObligationHonored {
		return unknown
	}
	return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk, Provenance: proofs.EvidenceFromLocalSSA}
}
