package lifecyclefacts

import (
	"go/token"
	"iter"
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Capture evidence keeps possible retention and unreadable handoff distinct
// from proven completion. Imported completion requires an immutable capture
// mapped to the exact argument; opaque or exhausted handoffs remain possible.

// ClosureRetainsValue reports whether a function literal may keep the value it
// captured, judged by reading its body.
//
// A named callee answers this from its summary, but a literal has no object
// and is never summarized, so the same question has to be asked of the body
// directly. The loose walk is the right one: this decides whether to suppress,
// so a literal that may keep the value must say yes. A literal with no body to
// read says yes for the same reason.
func (evidence *LifecycleEvidence) ClosureRetainsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || len(function.Blocks) == 0 {
		return true
	}
	retentions := evidence.retentionQueries()
	for uses := range capturedTargetUses(function, closure, target) {
		for _, held := range uses {
			if retentions.retainedAnywhere(evidence.pass, function, held) {
				return true
			}
		}
	}
	return false
}

func (evidence *LifecycleEvidence) retentionQueries() *retentionCache {
	if evidence.retentions == nil {
		evidence.retentions = newRetentionCache()
		// A consumer owns the prerequisite result, not the prerequisite's
		// object-fact namespace. Fix this lookup for the cache's whole life.
		evidence.retentions.lookup = func(call ssa.Instruction) (Fact, bool) { return factFor(evidence.pass, call) }
	}
	return evidence.retentions
}

// ClosureHandsValueToUnreadableCallee reports whether a literal passes the
// value it captured to a callee whose body this pass cannot read.
//
// Reading a literal's body decides whether the literal is transparent, and a
// transparent literal leaves the resource owned by the enclosing function. That
// is only sound for a body the pass can judge. A capture is a cell the body
// loads first, so the argument at such a call is the load rather than the
// value the caller acquired, and a summary matched on exact value identity
// never recognizes it: block/spirit closes rows through utils.CloseAndLog
// inside a deferred literal, and the release went uncredited while the literal
// was judged transparent.
//
// Rather than credit a release it cannot see, say the literal is not readable
// and let the caller keep the old opaque answer.
func (evidence *LifecycleEvidence) ClosureHandsValueToUnreadableCallee(
	closure *ssa.MakeClosure, target ssa.Value,
) bool {
	return evidence.ClosureHandsValueToUnreadableCalleeWithin(closure, target, nil)
}

// ClosureHandsValueToUnreadableCalleeWithin charges the capture and body census
// to budget. Exhaustion preserves possible opaque consumption; it never proves
// that the callback leaves target with its caller.
func (evidence *LifecycleEvidence) ClosureHandsValueToUnreadableCalleeWithin(
	closure *ssa.MakeClosure, target ssa.Value, budget *proofs.SearchBudget,
) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || len(function.Blocks) == 0 {
		return true
	}
	for held := range capturedTargetUsesWithin(function, closure, target, budget) {
		for instruction := range ssaflow.InstructionsWithin(function, budget) {
			if callHandsValueToUnreadableCallee(instruction, held, budget) {
				return true
			}
		}
		if budget.Exhausted() {
			return true
		}
	}
	return budget.Exhausted()
}

// callHandsValueToUnreadableCallee reports whether the instruction passes one
// of the held values to a callee with no body here. A dynamic callee and a
// callee in another package both qualify: go vet analyses one package at a
// time, so an imported body is absent and only its summary is available.
func callHandsValueToUnreadableCallee(instruction ssa.Instruction, held []ssa.Value, budget *proofs.SearchBudget) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	callee := common.StaticCallee()
	if callee != nil && len(callee.Blocks) > 0 {
		return false
	}
	for _, argument := range common.Args {
		for _, value := range held {
			if !budget.Spend() {
				return true
			}
			if heapmodel.MayAlias(argument, value) {
				return true
			}
		}
	}
	return false
}

// Select the same possible capture provenance for retention and unreadable
// handoff queries. Keep each capture's uses together and stop when the caller
// has its witness; neither query needs to inspect unrelated later captures.
func capturedTargetUses(function *ssa.Function, closure *ssa.MakeClosure, target ssa.Value) iter.Seq[[]ssa.Value] {
	return capturedTargetUsesWithin(function, closure, target, nil)
}

func capturedTargetUsesWithin(
	function *ssa.Function, closure *ssa.MakeClosure, target ssa.Value, budget *proofs.SearchBudget,
) iter.Seq[[]ssa.Value] {
	return func(yield func([]ssa.Value) bool) {
		for captured := range ssaflow.ClosureBindingPairsWithin(function, closure, budget) {
			if !heapmodel.CapturedBindingMatches(captured.Binding, target) &&
				!heapmodel.ValueDerivesFrom(captured.Binding, target) {
				continue
			}
			if !yield(capturedUsesWithin(captured.Free, budget)) {
				return
			}
		}
	}
}

// capturedUses returns the values a literal's body actually handles for a
// captured variable. A capture is a cell, and the body loads it before use, so
// asking the retention walk about the cell alone finds nothing: that walk
// matches values exactly, unlike the traversal that follows a value forward.
func capturedUses(free *ssa.FreeVar) []ssa.Value {
	return capturedUsesWithin(free, nil)
}

func capturedUsesWithin(free *ssa.FreeVar, budget *proofs.SearchBudget) []ssa.Value {
	uses := []ssa.Value{free}
	if free.Referrers() == nil {
		return uses
	}
	for _, reference := range *free.Referrers() {
		if !budget.Spend() {
			return uses
		}
		if load, ok := reference.(*ssa.UnOp); ok && load.Op == token.MUL && load.X == free {
			uses = append(uses, load)
		}
	}
	return uses
}

// factOwnsImmutableCapturedArgument maps an imported fact back through one
// literal capture. The capture cell must contain only target: if the enclosing
// function can replace it before the literal runs, the load in the literal no
// longer proves which value the imported callee receives. block/spirit closes
// rows through an imported CloseAndLog helper inside a deferred literal:
// https://github.com/block/spirit/blob/c554eae8c56166ad9199fc73556b29ed581ca575/pkg/checksum/single.go#L493-L503
func factOwnsImmutableCapturedArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer proofs.Observer) bool {
	if instruction == nil {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	function := instruction.Parent()
	if common == nil || function == nil || function.Parent() == nil {
		return false
	}
	for _, block := range function.Parent().Blocks {
		for _, candidate := range block.Instrs {
			closure, ok := candidate.(*ssa.MakeClosure)
			if !ok || closure.Fn != function {
				continue
			}
			for _, captured := range ssaflow.ClosureBindingPairs(function, closure) {
				if !immutableCapturedTarget(captured.Binding, target, closure, observer) {
					continue
				}
				for index, argument := range common.Args {
					if mask.contains(index) && slices.ContainsFunc(capturedUses(captured.Free), func(held ssa.Value) bool {
						return heapmodel.DefinitelySameValue(argument, held)
					}) {
						return true
					}
				}
			}
		}
	}
	return false
}

func immutableCapturedTarget(binding, target ssa.Value, observation ssa.Instruction, observer proofs.Observer) bool {
	storage := heapmodel.NewStorage(proofs.NewSearchBudget(proofs.QueryBudget).Observed(observer))
	if storage.Same(binding, target).Proven() {
		return true
	}
	stored := storage.StableContent(binding, observation)
	return stored.Proven() && storage.Same(stored.Value, target).Proven()
}

func (evidence *LifecycleEvidence) capturedImportedCompletion(request EvidenceRequest) Proof {
	if request.Completion == nil || request.SelectMask == nil {
		return Proof{}
	}
	common := ssaflow.InstructionCall(request.Instruction)
	if common == nil {
		return Proof{}
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return Proof{}
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || len(function.Blocks) == 0 {
		return Proof{}
	}
	completes := func(instruction ssa.Instruction) bool {
		fact, summarized := factFor(evidence.pass, instruction)
		return summarized && factOwnsImmutableCapturedArgument(instruction, request.Target, request.SelectMask(fact), evidence.probe.Observer())
	}
	if !lifecycle.MethodCallCoverage(function, completes, request.Completion.Coverage, nil) {
		return Proof{}
	}
	return importedProof(reasonLifecycleSummaryCapturedArgument, requestedMethod(request))
}
