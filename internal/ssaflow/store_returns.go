package ssaflow

import (
	"go/token"
	"iter"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

func ReturnedValueOwnsValue(returned *ssa.Return, value ssa.Value) bool {
	return newOwnershipSearch(nil).returnedValueOwnsValue(returned, value)
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
	return newOwnershipSearch(summarized).returnedValueOwnsValue(returned, value)
}

// ownershipSearch carries the cycle guard and the summary hook through the
// walk that asks whether a returned value holds another value.
type ownershipSearch struct {
	seen       map[ownershipPair]bool
	summarized ReturnsOwner
}

func newOwnershipSearch(summarized ReturnsOwner) *ownershipSearch {
	return &ownershipSearch{seen: map[ownershipPair]bool{}, summarized: summarized}
}

type ownershipPair struct {
	aggregate ssa.Value
	value     ssa.Value
}

func (search *ownershipSearch) returnedValueOwnsValue(returned *ssa.Return, value ssa.Value) bool {
	for _, result := range returned.Results {
		if SameValue(result, value) || search.aggregateStoresValue(result, value) {
			return true
		}
	}
	return false
}

func (search *ownershipSearch) aggregateStoresValue(aggregate, value ssa.Value) bool {
	pair := ownershipPair{aggregate: aggregate, value: value}
	if aggregate == nil || search.seen[pair] {
		return false
	}
	if SameValue(aggregate, value) {
		return true
	}
	search.seen[pair] = true
	// A constructor with a fast path returns the argument itself once it is
	// already the type it would wrap, as bufio.NewReaderSize does with
	// rd.(*Reader). The assertion selects the same object, so the caller
	// receives back what it passed in and still owns it.
	if inner, ok := UnwrapTransparentValue(
		aggregate,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface|
			TransparentTypeAssert,
	); ok {
		return search.aggregateStoresValue(inner, value)
	}
	switch typed := aggregate.(type) {
	case *ssa.Call:
		if search.callAggregateStoresValue(typed, value) {
			return true
		}
	case *ssa.Phi:
		if search.anyAggregateStoresValue(typed.Edges, value) {
			return true
		}
	case *ssa.Slice:
		return search.aggregateStoresValue(typed.X, value)
	case *ssa.MakeClosure:
		// A returned callback that captured the value keeps it alive and is the
		// only thing that can still release it, so the caller receives the
		// obligation with the callback.
		for _, binding := range typed.Bindings {
			if CapturedBindingMatches(binding, value) || search.aggregateStoresValue(CapturedBindingValue(binding), value) {
				return true
			}
		}
	case *ssa.UnOp:
		// Copying the owner by value, as in ephemeral(*cmd), carries the same
		// process or handle state, so returning the copy transfers it. A struct
		// literal returned by value is likewise loaded from the local that
		// assembled it, so the load carries whatever that local's fields hold.
		if typed.Op == token.MUL && (SameValue(typed.X, value) || search.aggregateStoresValue(typed.X, value)) {
			return true
		}
	}
	if _, ok := aggregate.(*ssa.Alloc); ok && search.addressStoresValue(aggregate, value) {
		return true
	}
	return search.aggregateReferrersStoreValue(aggregate, value)
}

func (search *ownershipSearch) callAggregateStoresValue(call *ssa.Call, value ssa.Value) bool {
	common := call.Common()
	if CallMatchesSymbol(common, syntax.Builtin("append")) && search.anyAggregateStoresValue(common.Args, value) {
		return true
	}
	callee := ResolvedCallee(common)
	if callee == nil {
		return false
	}
	for index, argument := range common.Args {
		if !search.aggregateStoresValue(argument, value) {
			continue
		}
		if index < len(callee.Params) && search.functionReturnsOwner(callee, callee.Params[index]) {
			return true
		}
		// A callee in another package has neither a body nor parameter values
		// here, so its summary is the only account of what it did with the
		// argument. Index the summary by the argument position, which is how
		// the summary counts parameters, receiver included.
		if search.summarized != nil && search.summarized(callee, index) {
			return true
		}
	}
	return false
}

func (search *ownershipSearch) anyAggregateStoresValue(aggregates []ssa.Value, value ssa.Value) bool {
	for _, aggregate := range aggregates {
		if search.aggregateStoresValue(aggregate, value) {
			return true
		}
	}
	return false
}

func (search *ownershipSearch) functionReturnsOwner(function *ssa.Function, value ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if ok && search.returnedValueOwnsValue(returned, value) {
				return true
			}
		}
	}
	return false
}

func (search *ownershipSearch) aggregateReferrersStoreValue(aggregate, value ssa.Value) bool {
	if aggregate.Referrers() == nil {
		return false
	}
	for _, reference := range *aggregate.Referrers() {
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

// callStoresValueIntoAggregate reports whether a call hands a callee both the
// aggregate and the value, and that callee stores the value into it.
//
// A constructor commonly delegates the assembly of the value it returns rather
// than writing the field itself: bufio.NewReader reaches its buffer through
// (*Reader).reset, and encoding/json's NewDecoder reaches its reader through
// jsontext.NewDecoder and then (*Decoder).Reset. The parameter is stored into
// the returned aggregate only inside those callees, so a search that stops at
// the caller's own stores concludes the callee kept the value for itself, and
// the caller is then wrongly credited with handing over the release. The
// helper is often unexported, so its summary is never exported and the answer
// has to come from its body, which is available here for a callee in the same
// package.
func (search *ownershipSearch) callStoresValueIntoAggregate(call ssa.CallInstruction, aggregate, value ssa.Value) bool {
	common := call.Common()
	if common == nil {
		return false
	}
	callee := ResolvedCallee(common)
	if callee == nil || len(callee.Blocks) == 0 {
		return false
	}
	// The aggregate may be handed over as a field of itself, as jsontext does
	// when Reset passes d.s to the state's own reset.
	holder := -1
	for index, argument := range common.Args {
		if index < len(callee.Params) &&
			(SameValue(argument, aggregate) || ValueIsAccessPathFrom(argument, aggregate)) {
			holder = index
			break
		}
	}
	if holder < 0 {
		return false
	}
	for index, argument := range common.Args {
		if index == holder || index >= len(callee.Params) {
			continue
		}
		// A callback that captured the value is not the value being stored.
		// Registering one, as t.Cleanup does, keeps the callback alive without
		// proving the callback releases anything, and the closure rules decide
		// that separately. Reading it as a store would accept a cleanup that
		// only conditionally releases.
		if _, closure := argument.(*ssa.MakeClosure); closure {
			continue
		}
		if !SameValue(argument, value) && !search.aggregateStoresValue(argument, value) {
			continue
		}
		if search.aggregateStoresValue(callee.Params[holder], callee.Params[index]) {
			return true
		}
	}
	return false
}

func (search *ownershipSearch) addressStoresValue(address ssa.Value, value ssa.Value) bool {
	for stored := range StoredInto(address) {
		if SameValue(stored, value) || search.aggregateStoresValue(stored, value) {
			return true
		}
	}
	return false
}

// StoredInto yields every value stored into address, into a field or element
// selected from it, or through a pointer loaded from it. It is the one walk
// for asking what an aggregate holds; callers supply the question about each
// stored value.
func StoredInto(address ssa.Value) iter.Seq[ssa.Value] {
	return func(yield func(ssa.Value) bool) {
		storedInto(address, yield, map[ssa.Value]bool{})
	}
}

func storedInto(address ssa.Value, yield func(ssa.Value) bool, seen map[ssa.Value]bool) bool {
	if address == nil || seen[address] || address.Referrers() == nil {
		return true
	}
	seen[address] = true
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if typed.Addr == address && !yield(typed.Val) {
				return false
			}
		case *ssa.FieldAddr, *ssa.IndexAddr:
			if !storedInto(typed.(ssa.Value), yield, seen) {
				return false
			}
		case *ssa.UnOp:
			// A returned owner may contain a pointer to a callback slot rather
			// than the callback directly. Follow the load so `*owner.cancel =
			// cancel` transfers the same obligation as `owner.cancel = cancel`.
			if typed.Op == token.MUL && !storedInto(typed, yield, seen) {
				return false
			}
		}
	}
	return true
}

// ReturnedResult returns the value a return statement hands back at index.
// A function with a defer keeps its results in cells so deferred calls can
// observe them: each return stores into the cell, runs the defers, and
// returns a load. The load names the cell, not the value, so this resolves
// it to the store made on the returning path, the last store into the cell
// in the return's own block.
