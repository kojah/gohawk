package resourcelifetime

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/resourcemodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Ownership evidence binds cleanup and publication to the caller's resource.
// Wrapper chains establish possible retention only at a foreign storage or
// retaining-call boundary; neither a wrapper's name nor its mere use transfers
// the obligation.

func localResourceOwners(function *ssa.Function, resource ssa.Value) []ssa.Value {
	var owners []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			stored := proveResourceStorage(instruction, resource, nil)
			if stored.Owner != nil && !stored.Proven() && !heapmodel.MayAliasAny(stored.Owner, owners) {
				owners = append(owners, stored.Owner)
			}
		}
	}
	return owners
}

// A helper can condition cleanup on an error it receives beside the
// resource. Unconditional completion cannot represent that relation, so a
// witnessed cleanup plus a correlated error is uncertainty, not proof of
// either release or a leak. The error is correlated when it is the one
// paired with this acquisition, or when the caller itself branches on it
// being nil after the call: then the caller's own paths split on the same
// value the helper's cleanup does, as in closeOnError(f, err) followed by
// if err != nil { return nil, err }; return f, nil.
// https://github.com/h44z/wg-portal/blob/eb44c8c4ff120f34c26b2415c47560f4fba0603c/internal/lowlevel/mikrotik.go#L267-L280
// Helpers that merely inspect the pair, receive an error the caller never
// tests again, or condition cleanup on a flag stay visible: a flag the
// caller does not branch on leaves the unreleased path feasible.
func (analysis *resourceAnalysis) pairedErrorHelperCleanup(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || !slices.Contains(common.Args, analysis.resource) ||
		!slices.ContainsFunc(common.Args, func(argument ssa.Value) bool { return analysis.correlatedError(instruction, argument) }) {
		return false
	}
	return lifecycle.ProveCompletion(lifecycle.CompletionRequest{
		Instruction: instruction,
		Target:      analysis.resource,
		Methods:     analysis.contract.cleanup,
		Coverage:    lifecycle.CoverageAnywhere,
		Budget:      analysis.budget(releaseSearchBudget),
	}).Proven()
}

// correlatedError reports whether an error handed to the helper call is the
// acquisition's paired error, or one the caller compares with nil after the
// call. Identity, not derivation: a wrapped error is a different value.
func (analysis *resourceAnalysis) correlatedError(call ssa.Instruction, argument ssa.Value) bool {
	if !syntax.IsErrorType(argument.Type()) {
		return false
	}
	if analysis.resource == ssaflow.CallResult(analysis.acquisition, 0) && argument == ssaflow.CallResult(analysis.acquisition, 1) {
		return true
	}
	for _, instruction := range ssaflow.InstructionsReachableAfter(call) {
		branch, ok := instruction.(*ssa.If)
		if !ok {
			continue
		}
		comparison, ok := branch.Cond.(*ssa.BinOp)
		if ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) &&
			(comparison.X == argument && ssaflow.DefinitelyNil(comparison.Y) || comparison.Y == argument && ssaflow.DefinitelyNil(comparison.X)) {
			return true
		}
	}
	return false
}

// An imported helper that releases every element of what it receives inside
// a loop exports that loop as a may-claim, as client-go's CloseAndRemove does
// for its variadic files; a visible helper's loop is found by the completion
// search itself. Either way the call is uncertain, never a release.
// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/staging/src/k8s.io/client-go/util/testing/remove_file.go#L25-L39
func (analysis *resourceAnalysis) importedLoopRelease(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) != 0 {
		return false
	}
	for index, argument := range common.Args {
		if released, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimReleasesInLoop); released &&
			analysis.carries(argument) {
			return true
		}
	}
	return false
}

func resourceLifecycleMethod(name string) bool {
	switch name {
	case "Close", "Kill", "Shutdown", "Stop", "Wait":
		return true
	default:
		return false
	}
}

// responseBodyAggregateHandoff recognizes the exact acquired Body inside an
// aggregate at the handoff. Projection stability is checked at the Body load,
// so a saved original survives a later replacement, but a replacement itself
// is not an owner. Current containment excludes overwritten aggregate fields
// and later stores. A send is uncertain ownership, never a cleanup guarantee.
// https://github.com/Contextualist/acp/blob/579b477d0281df41ab8753a7cbcb8f7807e52e2c/pkg/pnet/p2p.go#L79-L91
func (analysis *resourceAnalysis) responseBodyAggregateHandoff(value ssa.Value, at ssa.Instruction) resourceProof {
	missing := resourceProof{State: ssaflow.EvidenceDisproven}
	if analysis.contract.family != "http" || !heapmodel.CanHoldReference(value.Type()) {
		return missing
	}
	budget := analysis.budget(ssaflow.QueryBudget)
	storage := heapmodel.NewStorage(budget)
	for _, load := range ssaflow.InstructionsOf[*ssa.UnOp](analysis.function) {
		if !budget.Spend() {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if lifecyclefacts.ResponseBodyField(load) == nil || !storage.Projection(load, analysis.resource, load).Proven() {
			continue
		}
		contained, known := heapmodel.ContainsAt(value, load, at)
		if known && contained {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonResponseBodyAggregateHandoff}
		}
	}
	return missing
}

func (analysis *resourceAnalysis) proveAggregateOwnerEscapeWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *ssaflow.SearchBudget,
) resourceProof {
	for index, argument := range common.Args {
		if !budget.Spend() {
			return aggregateEscapeProof(false, budget)
		}
		candidate := analysis.proveAggregateArgumentWithin(argument, instruction, budget)
		if candidate.State == ssaflow.EvidenceUnknown {
			return candidate
		}
		if !candidate.Proven() {
			continue
		}
		// Dependence on the resource alone does not establish that a returned
		// wrapper is an owning aggregate. Require a proven store by this callee
		// before treating such a value as published through the next helper.
		direct := analysis.proveCarriedDirectlyWithin(argument, budget)
		if direct.State == ssaflow.EvidenceUnknown {
			return direct
		}
		if direct.Proven() {
			stored, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimStores)
			if !stored {
				continue
			}
		}
		// A parameter-level retention fact also makes its nested contents
		// uncertain. Variadic values stored for later callbacks are a common
		// example; a wrapper around that aggregate can retain it too.
		// A clear retention bit is not a purity proof.
		// https://github.com/rusq/slackdump/blob/f7319928b0993b23d7e9bd8af5e4c69b6f1d2af4/internal/convert/filecopy_test.go#L92-L106
		// https://github.com/Mmx233/BitSrunLoginGo/blob/a744f312b3835f329eb98e45c8d19bc2a5b7d4c0/internal/config/log.go#L50-L60
		if retained, _ := analysis.evidence.ArgumentRetained(instruction, index); retained {
			return aggregateEscapeProof(true, budget)
		}
		if syntax.PointerStruct(argument.Type()) == nil {
			continue
		}
		effects := analysis.evidence.CallEffects(instruction, argument)
		if effects.Proven() {
			if effects.Effects&(ssaflow.EffectRetain|ssaflow.EffectAsync) != 0 {
				return aggregateEscapeProof(true, budget)
			}
			continue
		}
		// A body this pass cannot read is judged by its summary alone. The
		// kept-contents claim is loose and indexed by path, so a summary
		// that keeps nothing at the path where this resource sits proves
		// that the resource cannot outlive the call through this callee,
		// while a helper that keeps or closes the other field says nothing
		// about this one. A resource whose position is unknown asks about
		// the whole aggregate. An unsummarized callee stays a boundary:
		// silence is not a proof.
		if kept, known := analysis.evidence.ContentsKeptAt(instruction, index, analysis.pathWithin(argument, instruction)); known && !kept {
			continue
		}
		return aggregateEscapeProof(true, budget)
	}
	return aggregateEscapeProof(false, budget)
}

// pathWithin returns the joined access path at which the resource is stored
// beneath the aggregate, or the empty path when its position is not known.
func (analysis *resourceAnalysis) pathWithin(aggregate ssa.Value, observation ssa.Instruction) string {
	relation := resourcemodel.ProveRelation(aggregate, analysis.resource, observation, analysis.budget(1000))
	if !relation.Proven() {
		return ""
	}
	return ssaflow.JoinAccessPath(relation.Relation.Path())
}

func (analysis *resourceAnalysis) proveAggregateArgumentWithin(
	argument ssa.Value, instruction ssa.Instruction, budget *ssaflow.SearchBudget,
) resourceProof {
	// The resource itself, or a load that resolves to it, is not an
	// aggregate holding the resource; only a genuine container is asked
	// whether it may escape.
	// The graph fallback observes the argument before the call, so a
	// summarized callee's later store cannot supply that observed relation.
	// Structural may-containment remains broader and proves no cleanup.
	// A returned wrapper can derive from the resource without being the
	// resource itself. Only a same-object argument is excluded here; the
	// wrapper may be retained by this callee and publish its contents.
	// https://github.com/bazelbuild/bazel-watcher/blob/ed00d96be0ce5b01aa2c43abbcd29172d4573091/cmd/ibazel/main.go#L178-L182
	if !budget.Spend() {
		return aggregateEscapeProof(false, budget)
	}
	if heapmodel.MayAlias(argument, analysis.resource) || analysis.carriedWithinClosure(argument) {
		return resourceProof{State: ssaflow.EvidenceDisproven}
	}
	contained := lifecycle.ProveMayContainValueAtWithin(argument, analysis.resource, instruction, budget)
	if contained.State == ssaflow.EvidenceUnknown {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if contained.Proven() {
		return aggregateEscapeProof(true, budget)
	}
	wrapper := analysis.provePossibleWrapperWithin(argument, 0, false, budget)
	if wrapper.State == ssaflow.EvidenceUnknown {
		return wrapper
	}
	return aggregateEscapeProof(wrapper.Proven(), budget)
}

func aggregateEscapeProof(found bool, budget *ssaflow.SearchBudget) resourceProof {
	if resourceFlowExhausted(budget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if found {
		return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonAggregateOwnerMayEscape}
	}
	return resourceProof{State: ssaflow.EvidenceDisproven}
}
