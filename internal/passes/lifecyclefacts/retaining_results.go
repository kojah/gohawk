package lifecyclefacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Retaining results extend owned results through wrappers. A constructor
// that acquires a resource and returns a value holding it, such as a logger
// over a file it opened, hands its caller an obligation that no method of
// the wrapper can settle: nothing on a *slog.Logger closes the file. The
// caller settles it only by keeping the wrapper somewhere that outlives it,
// handing it to code that does, or returning it in turn. The defect, when
// there is one, is a caller that drops the wrapper, so the obligation moves
// to the call site.
//
// The proof is as strict as the one for owned results. Every step from the
// resource to the result must be a call whose imported summary proves that
// its result holds the argument on every return (ReturnedOwner); a may-hold
// wrapper such as bufio.NewWriter, or a helper of this package, whose summary
// is still being built, declines. The resource and each wrapper may be
// converted to an interface, merged by a phi, or compared with nil, and
// nothing else: a method call, a store, a capture, a cleanup, or a second
// wrapper over the same value declines. Every normal return must then hand
// back a wrapper holding the resource, or nil results only, so the caller
// never inherits an obligation for a value the constructor did not build.

// maxRetainingChain bounds how many wrappers may nest between the resource
// and the result, as in slog.New(slog.NewTextHandler(file, nil)).
const maxRetainingChain = 4

// retainingResults returns the mask of result positions that hold, through
// a chain of proven wrappers, a fresh resource acquired in this function.
func retainingResults(pass *analysis.Pass, function *ssa.Function) ResultMask {
	var retaining ResultMask
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if acquired, ok := instruction.(ssa.Value); ok {
				if index, ok := retainingResultOf(pass, function, acquired); ok {
					retaining |= resultMaskFor(index)
				}
			}
		}
	}
	return retaining
}

// retainingResultOf reports the result position through which function
// hands acquired to its caller inside a proven wrapper.
func retainingResultOf(pass *analysis.Pass, function *ssa.Function, acquired ssa.Value) (int, bool) {
	if !acquiredResource(pass, acquired) || !freshlyAcquired(pass, acquired) || retainedOutsideResult(function, acquired) {
		return -1, false
	}
	walk := retentionWalk{pass: pass, index: -1, seen: map[ssa.Value]bool{}}
	if !walk.feedsReturn(acquired, 0) || walk.index < 0 || !returnedOwnerOnEveryReturn(pass, function, acquired) {
		return -1, false
	}
	return walk.index, true
}

// retentionWalk follows the resource forward through wrapper constructors
// and records the one result position the outermost wrapper reaches.
type retentionWalk struct {
	pass  *analysis.Pass
	index int
	seen  map[ssa.Value]bool
}

func (walk *retentionWalk) feedsReturn(value ssa.Value, depth int) bool {
	if walk.seen[value] {
		return true
	}
	walk.seen[value] = true
	if value.Referrers() == nil {
		return true
	}
	for _, reference := range *value.Referrers() {
		if !walk.useFeedsReturn(value, reference, depth) {
			return false
		}
	}
	return true
}

func (walk *retentionWalk) useFeedsReturn(value ssa.Value, use ssa.Instruction, depth int) bool {
	switch typed := use.(type) {
	case *ssa.Return:
		// The resource itself returned is an owned result, not a retained
		// one; only a wrapper may reach the return here.
		position := returnedPosition(typed, value)
		if depth == 0 || position < 0 || walk.index >= 0 && walk.index != position {
			return false
		}
		walk.index = position
		return true
	case *ssa.MakeInterface, *ssa.ChangeInterface, *ssa.Phi:
		return walk.feedsReturn(typed.(ssa.Value), depth)
	case *ssa.BinOp:
		return typed.Op == token.EQL || typed.Op == token.NEQ
	case *ssa.Call:
		return depth < maxRetainingChain && walk.wrapperHolds(typed, value) && walk.feedsReturn(typed, depth+1)
	case *ssa.DebugRef:
		return true
	}
	return false
}

// wrapperHolds reports whether call is a single-result constructor whose
// imported summary proves that its result holds value, passed exactly once.
func (walk *retentionWalk) wrapperHolds(call *ssa.Call, value ssa.Value) bool {
	if _, tuple := call.Type().(*types.Tuple); tuple || call.Common().IsInvoke() {
		return false
	}
	position := -1
	for index, argument := range call.Common().Args {
		if argument == value {
			if position >= 0 {
				return false
			}
			position = index
		}
	}
	if position < 0 {
		return false
	}
	fact, ok := importFact(walk.pass, call)
	return ok && fact.Claim(ClaimReturnsOwner).contains(position)
}

// RetainingResult reports whether the call's static callee is summarized as
// returning a wrapper that holds a fresh resource, and at which result. No
// cleanup method is returned: the caller cannot release the resource through
// the wrapper, only keep, hand over, or drop it.
func (evidence *LifecycleEvidence) RetainingResult(call *ssa.Call) (int, bool) {
	fact, ok := factFor(evidence.pass, call)
	if !ok || fact.Must.RetainingResults == 0 {
		return -1, false
	}
	for index := range call.Common().Signature().Results().Len() {
		if fact.Must.RetainingResults.contains(index) {
			evidence.emit(EvidenceRequest{Instruction: call, Target: call}, Proof{Proof: ssaflow.Proof{
				State: ssaflow.EvidenceProven, Provenance: ssaflow.EvidenceFromImportedFact,
			}, SummaryReason: reasonRetainingResultContract})
			return index, true
		}
	}
	return -1, false
}

// RetainingResultClaimed reports whether the function's own summary claims
// the result at index as a retaining result. The constructor asks this at a
// return that hands a wrapper over the resource to that result, so its
// handover and the caller's obligation come from the same claim.
func (evidence *LifecycleEvidence) RetainingResultClaimed(function *ssa.Function, index int) bool {
	summaries, _ := evidence.pass.ResultOf[Analyzer].(Summaries)
	fact, ok := summaries[function]
	return ok && fact.Must.RetainingResults.contains(index)
}
