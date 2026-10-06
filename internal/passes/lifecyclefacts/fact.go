package lifecyclefacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"

	"golang.org/x/tools/go/ssa"
)

// Fact is the compact cross-package ownership summary exported for a
// function. Its claims are grouped by polarity, so every use site states
// which kind it reads: a Must claim holds on every normal return and a
// consumer may settle on it, while a May claim over-approximates and a
// consumer may only treat it as unknown, never as settled. This package is
// internal analysis infrastructure, not a public extension API.
type Fact struct {
	Must MustClaims
	May  MayClaims
	// Discharges are the exact cleanup claims, unconditional and conditional
	// in one list: which method is called, on which parameter, at which
	// access path beneath it, on every normal return of the case Condition
	// names. A discharge with an empty condition is a Must claim; any other
	// is a summary case, a positive guarantee a caller selects only when it
	// can check the condition, so it never widens an unconditional claim and
	// a missing case does not establish no effect. An empty path means the
	// parameter itself (MethodMask); InvokeMethod means calling a function
	// parameter at all, and SynchronousInvokeMethod calling it in the same
	// goroutine before returning; a field or element path lets a caller match
	// the resource it stored there rather than any resource the argument
	// contains. See conditional.go for the cases.
	Discharges []Discharge
	// Heap is the projection of the function's points-to graph onto what a
	// caller can name: where each parameter, result, and global slot may
	// point at exit, how each object escaped or was released, what was
	// read, and where the projection was cut. See heap.go.
	Heap *heapmodel.HeapSummary
	// ReturnedCleanup relates an invoked callback result to an exact factory
	// parameter or sibling result. Merely returning the callback does not clean up.
	ReturnedCleanup *ReturnedCleanupSummary
	// signature is the summarized function's signature. It is attached
	// whenever a fact is produced or read for a known function and is never
	// serialized. It supplies the type gates the heap projection cannot see
	// when the transfer claims are read from Heap; see heap.go.
	signature *types.Signature
}

// MustClaims hold on every normal return of the function. The transfer
// claims of the same polarity, ReturnedOwner, Stored, and ReceiverStore, are
// read from Heap rather than stored; see heap.go.
type MustClaims struct {
	// ReturnedView narrows ReturnedOwner: the parameter is stored in the
	// returned struct, but no method of that type releases the field, so the
	// caller keeps the obligation. See returned_views.go.
	ReturnedView ParameterMask
	// OwnedFields and ReleasedFields are indexed by struct field, not
	// parameter; see fields.go for the constructor and method summaries.
	OwnedFields    FieldMask
	ReleasedFields FieldMask
	// OwnedResults is indexed by result position: the function hands back a
	// fresh resource it acquired itself, and the caller owes its cleanup.
	// See owned_results.go for the freshness the proof requires.
	OwnedResults ResultMask
	// RetainingResults is indexed by result position: the function hands
	// back a wrapper that holds a fresh resource it acquired, and the caller
	// must keep, hand over, or return that wrapper. See retaining_results.go.
	RetainingResults ResultMask
}

// MayClaims over-approximate what the function might do. A set bit never
// proves an effect happened, and a clear bit never proves it did not. The
// retention claims of the same polarity, Retained and Kept, are read from
// Heap rather than stored; see heap.go.
type MayClaims struct {
	// LoopReleased marks parameters whose derived values the callee releases
	// inside a loop, as a variadic close helper does to each of its files.
	// Which element an iteration releases is decided by iteration, so a
	// consumer treats the call as unknown, never as settled.
	LoopReleased ParameterMask
}

// KeptParameters returns the parameters with a kept-contents claim at any
// path.
func (fact *Fact) KeptParameters() ParameterMask {
	var mask ParameterMask
	for _, kept := range fact.Kept() {
		mask |= parameterMaskFor(kept.Parameter)
	}
	return mask
}

// keepsContentsAt reports whether the summary claims that the contents at
// path beneath the parameter at index may be kept beyond the call. The
// empty path asks about the whole parameter.
func (fact *Fact) keepsContentsAt(index int, path string) bool {
	var kept []string
	for _, claim := range fact.Kept() {
		if claim.Parameter == index {
			kept = append(kept, claim.Path)
		}
	}
	return keepsContentsAt(kept, path)
}

// Claim names what a summary can say about one parameter. The masks it
// selects are the vocabulary every proof shares, so a proof asks for the claim
// it needs rather than knowing which fields spell it. Releasing is a union
// because the settling action depends on the resource: a file is closed, a
// ticker stopped, a transaction committed or rolled back.
type Claim uint8

const (
	ClaimReturnsOwner Claim = iota
	ClaimReturnsView
	ClaimRetains
	ClaimStores
	ClaimReleases
	ClaimSynchronouslyInvokes
	ClaimReleasesInLoop
	// ClaimAsynchronouslyExposes is a may-claim derived from positive heap
	// escape effects. It cannot establish release or transfer on every return.
	ClaimAsynchronouslyExposes
)

// Claim returns the parameters this summary makes the claim about.
func (fact *Fact) Claim(claim Claim) ParameterMask {
	switch claim {
	case ClaimReturnsOwner:
		return fact.ReturnedOwner()
	case ClaimReturnsView:
		return fact.Must.ReturnedView
	case ClaimRetains:
		return fact.Retained()
	case ClaimStores:
		return fact.Stored()
	case ClaimReleases:
		return fact.DischargedParameters()
	case ClaimSynchronouslyInvokes:
		return fact.SynchronouslyInvoked()
	case ClaimReleasesInLoop:
		return fact.May.LoopReleased
	case ClaimAsynchronouslyExposes:
		return fact.heapClaim(func(index int) bool { return asynchronouslyExposed(fact.Heap, index) })
	}
	return 0
}

// ParameterMask is a set of SSA parameter positions in a lifecycle summary.
type ParameterMask uint64

// parameterMaskFor returns the mask containing index, or an empty mask when
// index cannot be represented by a lifecycle summary.
func parameterMaskFor(index int) ParameterMask {
	if index < 0 || index >= 64 {
		return 0
	}
	return ParameterMask(1) << index
}

// contains reports whether mask contains index.
func (mask ParameterMask) contains(index int) bool {
	return mask&parameterMaskFor(index) != 0
}

// FieldMask is a set of struct field indices of a result or receiver type.
// It is a separate type so a field bit is never tested as a parameter.
type FieldMask uint64

func fieldMaskFor(index int) FieldMask { return FieldMask(parameterMaskFor(index)) }

func (mask FieldMask) contains(index int) bool { return mask&fieldMaskFor(index) != 0 }

// ResultMask is a set of result positions of a function signature.
type ResultMask uint64

func resultMaskFor(index int) ResultMask { return ResultMask(parameterMaskFor(index)) }

func (mask ResultMask) contains(index int) bool { return mask&resultMaskFor(index) != 0 }

// Summaries is the pass result: the summary of every exported source function
// in the package plus the imported summary of every static callee.
type Summaries map[*ssa.Function]Fact

// SummarizedPackage marks a package whose exported functions with bodies
// were all summarized. A function of such a package with no summary of its
// own was proven to do nothing with its parameters; only a function of a
// package without the marker, or one listed as bodiless, is unknown. It
// exists so an empty summary need not be serialized for every function of
// every dependency, which the analysis framework would otherwise decode
// once per dependent package.
type SummarizedPackage struct {
	Bodiless []string
}

// empty reports whether the summary claims nothing.
func (fact *Fact) empty() bool {
	masks := fact.ReturnedOwner() | fact.Must.ReturnedView |
		fact.Retained() | fact.Stored() | fact.May.LoopReleased | fact.ReceiverStore()
	indexed := uint64(fact.Must.OwnedFields) | uint64(fact.Must.ReleasedFields) | uint64(fact.Must.OwnedResults) | uint64(fact.Must.RetainingResults)
	return masks == 0 && indexed == 0 && len(fact.Kept()) == 0 && len(fact.Discharges) == 0 &&
		(fact.ReturnedCleanup == nil || len(fact.ReturnedCleanup.Effects) == 0) &&
		fact.heapEmpty()
}

// heapEmpty reports whether the heap projection claims nothing a caller
// must react to: no edges, no truncation, and no effect beyond handing an
// object to a call. Reads are not exported, and a call escape alone says
// nothing an importer acts on: the summary's truncation already carries
// what an unresolved callee may have done, and a resolved one carries its
// own summary.
func (fact *Fact) heapEmpty() bool {
	if fact.Heap == nil {
		return true
	}
	if len(fact.Heap.Edges) != 0 || len(fact.Heap.Truncated) != 0 || len(fact.Heap.Requires) != 0 {
		return false
	}
	for _, effect := range fact.Heap.Effects {
		if effect.Release != "" || effect.Escape&^heapmodel.HeapEscapedCall != 0 {
			return false
		}
	}
	return true
}

// cleanupMethods is the lifecycle method vocabulary: a call of one of them
// on a parameter, on every return, is recorded as a discharge. It is the one
// catalog shared by summarization, loop releases, conditional effects, and
// the question of whether a type can release what it holds.
var cleanupMethods = []string{"Close", "Finalize", "Release", "Shutdown", "Stop", "Wait", "Commit", "Rollback"}
