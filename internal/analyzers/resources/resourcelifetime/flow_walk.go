package resourcelifetime

import (
	"strconv"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/resourcemodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// The resource walk keeps activation, cleanup and uncertainty on each path.
// Shared traversal and guard mechanics charge the existing candidate pool;
// incomplete evidence never retains a diagnostic witness or a release claim.

type resourceFlowState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	index       int
	obligation  resourcemodel.Obligation
	// guards are the branch outcomes this path has established. An edge
	// that contradicts one is unknown, never pruned: a guard read from a
	// cell could have changed through a pointer the analysis does not see,
	// and even a stable guard's contradiction only declines to report through
	// a path the analysis cannot rule out. Mutagen guards a profiler's
	// creation and its finalization on one address-taken flag, and fortio a
	// profile file's creation and its close on one option field:
	// https://github.com/mutagen-io/mutagen/blob/6ccfeaaf4dfd261e59ef9aac56e3c157b62e605b/tools/scan_bench/main.go#L140-L172
	// https://github.com/fortio/fortio/blob/5c19725ff61c9f7ad944b91ec32d96a399341d87/fhttp/httprunner.go#L199-L215
	guards ssaflow.PathGuards
}

type resourceFlowKey struct {
	location   ssaflow.FlowLocationKey
	obligation resourcemodel.Obligation
}

// proveResourceFlow owns setup, coverage and cutoff availability together.
// Resource-specific activation and contradiction policy stays in this walk;
// the shared engines supply traversal mechanics, never diagnostic policy.
func (analysis *resourceAnalysis) proveResourceFlow(errorValue ssa.Value) resourceLifetimePolicyResult {
	budget := analysis.budget(resourcePoolBudget)
	index := ssaflow.InstructionIndexWithin(analysis.acquisition, budget)
	if resourceFlowExhausted(budget) {
		return unknownResourceLifetime(resourceReasonBudgetExhausted)
	}
	if index < 0 {
		return unknownResourceLifetime(resourceReasonAcquisitionLocationUnknown)
	}
	// Retain reachability's existing per-question cap while charging all of its
	// traversal and feasibility work to the candidate pool.
	reachBudget := analysis.budget(ssaflow.SummaryBudget)
	reachable := analysis.acquisitionReachable(reachBudget)
	if resourceFlowExhausted(budget) || resourceFlowExhausted(reachBudget) {
		return unknownResourceLifetime(resourceReasonBudgetExhausted)
	}
	if !reachable {
		return acceptedResourceLifetime(resourceReasonAcquisitionUnreachable)
	}
	// Begin after acquisition; predecessor and obligation state distinguish
	// error edges from paths that still owe cleanup.
	guards := ssaflow.GuardsDominatingWithin(analysis.acquisition, budget)
	initial := []resourceFlowState{{block: analysis.acquisition.Block(), index: index + 1, obligation: resourcemodel.Acquired(), guards: guards}}
	opaque, leaks, incomplete := false, false, false
	ssaflow.WalkStatesWithin(initial, func(state resourceFlowState) resourceFlowKey { return resourceStateKey(state, budget) },
		func(state resourceFlowState) ([]resourceFlowState, bool) {
			state, leaks = advanceResourceState(analysis, state, budget)
			if resourceFlowExhausted(budget) || leaks {
				return nil, false
			}
			opaque = opaque || state.obligation.Unknown()
			edges := resourceSuccessorStates(analysis, state, errorValue, budget)
			incomplete = edges.State == ssaflow.EvidenceUnknown
			return edges.states, !incomplete
		}, budget)
	// Cutoff cannot retain a leak witness or an exact release claim, even when
	// a nested classifier or edge callback exhausted the candidate pool.
	if resourceFlowExhausted(budget) || incomplete {
		analysis.leak = nil
		return unknownResourceLifetime(resourceReasonBudgetExhausted)
	}
	if leaks {
		result := reportedResourceLifetime(resourceReasonUnownedReturn)
		result.leak = analysis.leak
		return result
	}
	if opaque {
		return unknownResourceLifetime(resourceReasonOpaqueConsumption)
	}
	return acceptedResourceLifetime(resourceReasonReleaseProven)
}

func resourceStateKey(state resourceFlowState, budget *ssaflow.SearchBudget) resourceFlowKey {
	return resourceFlowKey{
		location:   ssaflow.FlowLocationKeyWithin(state.block, state.predecessor, state.index, state.guards, budget),
		obligation: state.obligation,
	}
}

// A sibling classifier query can exhaust the shared parent without spending
// through the walk's child again. Both limits must still invalidate its proof.
func resourceFlowExhausted(budget *ssaflow.SearchBudget) bool {
	return budget.Exhausted() || budget.PoolExhausted()
}

func advanceResourceState(analysis *resourceAnalysis, state resourceFlowState, budget *ssaflow.SearchBudget) (resourceFlowState, bool) {
	// A release or transfer anywhere before a return settles the path. An
	// opaque consumption does not settle it but removes the proof: the
	// return is then neither owned nor a defect.
	for _, instruction := range state.block.Instrs[state.index:] {
		if !budget.Spend() {
			return state, false
		}
		state.guards = state.guards.AfterWithin(instruction, budget)
		if resourceFlowExhausted(budget) {
			return state, false
		}
		switch analysis.action(instruction) {
		case actionSettled:
			state.obligation = state.obligation.Discharged()
		case actionUnknown:
			state.obligation = state.obligation.Uncertain()
		case actionNone:
		}
		if resourceFlowExhausted(budget) {
			return state, false
		}
		// A call that never returns, whether os.Exit or a project's own fatal
		// wrapper the summaries prove, ends this path with nothing to release.
		terminated := ssaflow.InstructionTerminatesWithin(instruction, analysis.summaries.TerminatesWithin(budget), budget)
		if resourceFlowExhausted(budget) {
			return state, false
		}
		if terminated {
			state.obligation = state.obligation.Absent()
			break
		}
		returned, ok := instruction.(*ssa.Return)
		if ok && analysis.probe.Enabled() {
			analysis.probe.Evidence(analysisTrace.Step{
				Reason: resourceReasonResourceReturnPath.String(), Outcome: analysisTrace.OutcomeObserved,
				Pos: returned.Pos(), Function: returned.Parent().String(),
				Details: map[string]string{
					"active":   strconv.FormatBool(state.obligation.Active()),
					"released": strconv.FormatBool(state.obligation.Settled()),
					"unknown":  strconv.FormatBool(state.obligation.Unknown()),
				},
			})
		}
		if ok && state.obligation.Unsettled() {
			proof := analysis.proveResourceReturn(returned, budget)
			if proof.state == ssaflow.EvidenceUnknown {
				return state, false
			}
			if proof.state == ssaflow.EvidenceProven {
				analysis.leak = proof.leak
				return state, true
			}
		}
	}
	return state, false
}

type resourceSuccessorsProof struct {
	resourceProof
	states []resourceFlowState
}

func resourceSuccessorStates(
	analysis *resourceAnalysis, state resourceFlowState, errorValue ssa.Value, budget *ssaflow.SearchBudget,
) resourceSuccessorsProof {
	pass, resource, optionalAcquisition, candidate := analysis.pass, analysis.resource, analysis.optional, analysis.candidate
	edges := analysis.successorPolicy().EdgesWithin(state.block, state.predecessor, state.guards, budget)
	if optionalAcquisition.Proven() && state.block == optionalAcquisition.merge && state.predecessor == optionalAcquisition.acquisitionBlock {
		acquired := optionalAcquisition.acquiredSuccessor
		guards, contradiction := state.guards.ExtendWithin(state.block, acquired, nil, budget)
		edges = []ssaflow.SuccessorEdge{{To: acquired, Guards: guards, Contradiction: contradiction}}
		traceOptionalAcquisition(pass, optionalAcquisition, candidate)
	}
	if resourceFlowExhausted(budget) {
		return unavailableResourceSuccessors()
	}
	result := make([]resourceFlowState, 0, len(edges))
	for _, edge := range edges {
		if !budget.Spend() {
			return unavailableResourceSuccessors()
		}
		successor := edge.To
		obligation := state.obligation
		branch := proveResourceSuccessBranch(pass, analysis.summaries, state.block, successor, errorValue, candidate,
			budget.Within(ssaflow.SummaryBudget))
		if branch.State == ssaflow.EvidenceUnknown {
			return unavailableResourceSuccessors()
		}
		if branch.Proven() {
			if !branch.success {
				obligation = obligation.Absent()
			}
		}
		presence := proveResourcePresenceBranch(state.block, state.predecessor, successor, resource, budget)
		if presence.Proven() && !presence.Present {
			obligation = obligation.Absent()
		}
		// Error and presence evidence change activation only on this edge.
		// Repeated guard contradictions instead retain an unknown obligation:
		// dropping it would turn unavailable path evidence into cleanup.
		guards, contradiction := edge.Guards, edge.Contradiction
		if contradiction != ssaflow.GuardConsistent {
			obligation = obligation.Uncertain()
			analysis.traceUncertainEdge(state.block, successor, resourceReasonRepeatedGuardEdgeUnknown)
		}
		rows := proveSQLRowsExhaustionEdge(state.block, successor, resource, budget.Within(ssaflow.QueryBudget))
		if rows.State == ssaflow.EvidenceUnknown {
			return unavailableResourceSuccessors()
		}
		if rows.Proven() {
			obligation = obligation.Uncertain()
			analysis.traceUncertainEdge(state.block, successor, resourceReasonRowsExhaustedEdgeUnknown)
		}
		if analysis.collection.releasedOnEdge(state.block, successor) {
			obligation = obligation.Discharged()
			analysis.traceCollectionReleased(state.block, successor)
		}
		// A conditional helper settles only the edge selected by its result.
		// Optional-acquisition phis retain their own stricter cleanup policy.
		if !obligation.Settled() && !optionalAcquisition.Proven() {
			if analysis.evidence.CompletionOnEdge(state.block, successor, lifecycle.CompletionRequest{
				Target: resource, Methods: analysis.contract.cleanup, Budget: analysis.budget(1000),
			}).Proven() {
				obligation = obligation.Discharged()
			}
		}
		if resourceFlowExhausted(budget) {
			return unavailableResourceSuccessors()
		}
		result = append(result, resourceFlowState{
			block: successor, predecessor: state.block, obligation: obligation, guards: guards,
		})
	}
	return resourceSuccessorsProof{resourceProof: resourceProof{State: ssaflow.EvidenceProven}, states: result}
}

func unavailableResourceSuccessors() resourceSuccessorsProof {
	return resourceSuccessorsProof{resourceProof: resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}}
}
