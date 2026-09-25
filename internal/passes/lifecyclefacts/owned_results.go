package lifecyclefacts

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Owned results are the direct-result counterpart of owned fields: a
// function that acquires a resource itself and hands it back as a result,
// rather than inside a struct, gives its caller the obligation to release
// it. The proof is deliberately strict about freshness. The value must come
// from an acquisition in this body, it must reach the return untouched, and
// nothing else may keep or release it: a resource stored in a registry, a
// receiver, or a returned closure, one that a method call or a helper has
// seen, or one that a cleanup can reach before the return, is declined. The
// cost is recall on helpers that prepare the resource before returning it;
// the gain is that a claimed owner is never a shared or already-closed one.

// OwnedDirectResult reports whether the call's static callee is summarized
// as returning a fresh resource directly, and returns that result's cleanup
// methods and index. The result type decides the cleanup: a concrete
// resource type or an io.Closer. A type this vocabulary cannot release
// yields false, because the caller cannot be asked for a cleanup it has no
// way to perform.
func (evidence *LifecycleEvidence) OwnedDirectResult(call *ssa.Call) ([]string, int, bool) {
	fact, ok := factFor(evidence.pass, call)
	if !ok || fact.Must.OwnedResults == 0 {
		return nil, 0, false
	}
	results := call.Common().Signature().Results()
	for index := range results.Len() {
		if !fact.Must.OwnedResults.contains(index) {
			continue
		}
		cleanup, ok := typeCleanup(results.At(index).Type())
		if !ok {
			return nil, 0, false
		}
		evidence.emit(EvidenceRequest{Instruction: call, Target: call}, Proof{Proof: ssaflow.Proof{
			State: ssaflow.EvidenceProven, Provenance: ssaflow.EvidenceFromImportedFact,
		}, SummaryReason: reasonOwnedResultContract})
		return cleanup, index, true
	}
	return nil, 0, false
}

// ownedResults returns the mask of result positions that hold, on every
// successful return, a fresh resource acquired in this function.
func ownedResults(pass *analysis.Pass, function *ssa.Function) ResultMask {
	var owned ResultMask
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			acquired, ok := instruction.(ssa.Value)
			if !ok || !acquiredResource(pass, acquired) || !freshlyAcquired(pass, acquired) || retainedOutsideResult(function, acquired) {
				continue
			}
			index, fresh := freshResultIndex(function, acquired)
			if fresh && returnedOwnerOnEveryReturn(pass, function, acquired) {
				owned |= resultMaskFor(index)
			}
		}
	}
	return owned
}

// freshlyAcquired reports whether the call that produced the value can be
// trusted to have acquired it. A call returning a resource type is not
// enough: a library helper may hand back a response whose body it already
// read and closed, and a wrapper may forward a private helper whose
// behaviour is unknown. Two shapes are trusted. The callee is declared in
// the package that defines the resource type, which is where a resource is
// constructed, as os.Open constructs *os.File and (*http.Client).Do
// constructs *http.Response. Or the callee's imported summary claims the
// result as owned, so the claim composes across package boundaries.
// https://github.com/quackduck/devzat/blob/2fb7d6f3b5c4b53bd9b0bd5bd8f8a6a5f2d2f9d1/twitter.go#L54-L75
func freshlyAcquired(pass *analysis.Pass, acquired ssa.Value) bool {
	call, index, ok := ssaflow.CallResultSource(acquired)
	if !ok {
		return false
	}
	callee := ssaflow.ResolvedCallee(call.Common())
	if callee == nil || callee.Object() == nil {
		return false
	}
	if packagePath, ok := resourcePackage(acquired.Type()); ok && syntax.DeclaredInPackage(callee.Object(), packagePath) {
		return true
	}
	// Only an imported summary is consulted: this package's own summaries
	// are still being built while this proof runs, and a claim must not
	// depend on the order in which its functions were summarized.
	fact, summarized := importFact(pass, call)
	return summarized && fact.Must.OwnedResults.contains(index)
}

// resourcePackage returns the package that defines the value's resource type.
func resourcePackage(value types.Type) (string, bool) {
	for _, entry := range resourceTypes() {
		if syntax.NamedType(value, entry.packagePath, entry.name) {
			return entry.packagePath, true
		}
	}
	return "", false
}

// freshResultIndex reports the result position the acquired value reaches
// through return-feeding uses only, and whether every use of it is such a
// use. A comparison with nil is allowed, and so is a cleanup call, whose
// ordering against the returns is checked separately; any other call,
// store, capture, or consumption makes the value something other than a
// fresh handoff.
func freshResultIndex(function *ssa.Function, acquired ssa.Value) (int, bool) {
	walk := freshnessWalk{index: -1, seen: map[ssa.Value]bool{}}
	if !walk.feedsReturn(acquired) || walk.index < 0 {
		return -1, false
	}
	// A cleanup that can run before a return of the value would hand the
	// caller an already released resource; decline rather than guess which
	// path was taken.
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if releasesAcquiredBeforeReturn(function, instruction, acquired) {
				return -1, false
			}
		}
	}
	return walk.index, true
}

// freshnessWalk follows an acquired value through the uses a fresh handoff
// may have and records the one result position it reaches.
type freshnessWalk struct {
	index int
	seen  map[ssa.Value]bool
}

func (walk *freshnessWalk) feedsReturn(value ssa.Value) bool {
	if walk.seen[value] {
		return true
	}
	walk.seen[value] = true
	if value.Referrers() == nil {
		return true
	}
	for _, reference := range *value.Referrers() {
		if !walk.useFeedsReturn(value, reference) {
			return false
		}
	}
	return true
}

func (walk *freshnessWalk) useFeedsReturn(value ssa.Value, use ssa.Instruction) bool {
	switch typed := use.(type) {
	case *ssa.Return:
		position := returnedPosition(typed, value)
		if position < 0 || walk.index >= 0 && walk.index != position {
			return false
		}
		walk.index = position
		return true
	case *ssa.MakeInterface, *ssa.ChangeInterface, *ssa.Phi:
		return walk.feedsReturn(typed.(ssa.Value))
	case *ssa.BinOp:
		return typed.Op == token.EQL || typed.Op == token.NEQ
	case *ssa.Call:
		return cleanupCallOn(typed.Common(), value)
	case *ssa.DebugRef:
		return true
	}
	return false
}

func returnedPosition(returned *ssa.Return, value ssa.Value) int {
	for position, result := range returned.Results {
		if result == value {
			return position
		}
	}
	return -1
}

// releasesAcquiredBeforeReturn reports whether the instruction is a cleanup
// call on the acquired value from which some return of that value is
// reachable.
func releasesAcquiredBeforeReturn(function *ssa.Function, instruction ssa.Instruction, acquired ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || !cleanupCallOn(common, acquired) {
		return false
	}
	for _, later := range ssaflow.InstructionsReachableAfter(instruction) {
		if returned, ok := later.(*ssa.Return); ok && returned.Parent() == function && returnedPosition(returned, acquired) >= 0 {
			return true
		}
	}
	return false
}

// cleanupCallOn reports whether the call invokes one of the value's cleanup
// methods on the value itself.
func cleanupCallOn(common *ssa.CallCommon, value ssa.Value) bool {
	cleanup, ok := typeCleanup(value.Type())
	if !ok || ssaflow.CallReceiver(common) != value {
		return false
	}
	return slices.Contains(cleanup, ssaflow.CallName(common))
}
