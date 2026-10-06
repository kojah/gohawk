package resourcelifetime

import (
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"

	proofs "github.com/kojah/gohawk/internal/proof"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Resource flow tracks an acquired value from the successful call edge to each
// feasible normal return. State records activation and release separately so
// error-only resources and path-specific cleanup do not create false leaks.

// evaluateResourceFlow collects acquisition evidence, asks the path proof and
// applies the bounded policy exclusions to its diagnostic witness.
func evaluateResourceFlow(
	pass *analysis.Pass,
	evidence *lifecyclefacts.LifecycleEvidence,
	call *ssa.Call,
	resource ssa.Value,
	contract resourceContract,
) resourceLifetimePolicyResult {
	// This is a policy exclusion, not proof that a compressor was finalized.
	// Keep it before candidate evidence and all flow queries, as in the entry
	// point's former gate, so moving the decision does not add analysis work.
	if memoryWriterExempt(call, contract) {
		return acceptedResourceLifetime(resourceReasonMemoryWriter)
	}
	evidence.ForCandidate(call.Pos())
	probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos())
	pool := proofs.NewSearchBudget(resourcePoolBudget).Observed(probe.Observer())
	// The paired error restricts the edges on which acquisition creates an
	// obligation. A shortened lookup must stop here, before missing evidence
	// can activate ownership on a failed acquisition edge.
	errorResult := proveAcquisitionErrorResultWithin(call, pool.Within(releaseSearchBudget))
	if errorResult.proof.State == proofs.EvidenceUnknown {
		return unknownResourceLifetime(errorResult.proof.Reason)
	}
	errorValue := errorResult.value
	if reason := httpAcquisitionBoundary(pass, call, pool.Within(releaseSearchBudget)); reason != resourceReasonNone {
		return unknownResourceLifetime(reason)
	}
	canceled := proveAcquisitionContextCanceledWithin(call, pool.Within(releaseSearchBudget))
	if canceled.State == proofs.EvidenceUnknown {
		return unknownResourceLifetime(canceled.Reason)
	}
	if canceled.Proven() {
		return acceptedResourceLifetime(resourceReasonCanceledAcquisition)
	}
	assertedError := proveAcquisitionErrorWithin(call, resource, errorValue, contract.packagePath == "net/http", pool.Within(releaseSearchBudget))
	if assertedError.State == proofs.EvidenceUnknown {
		return unknownResourceLifetime(assertedError.Reason)
	}
	if assertedError.Proven() {
		return acceptedResourceLifetime(resourceReasonReleaseProven)
	}
	// Optional acquisition correlates the resource and its paired error on
	// the same diamond edge. Only a complete correlation may replace the
	// resource input used by owner discovery and the subsequent path proof.
	optionalAcquisition := proveOptionalAcquisitionWithin(call, resource, errorValue, pool.Within(releaseSearchBudget))
	if optionalAcquisition.proof.State == proofs.EvidenceUnknown {
		return unknownResourceLifetime(optionalAcquisition.proof.Reason)
	}
	if optionalAcquisition.Proven() {
		resource = optionalAcquisition.resourcePhi
	}
	analysis := &resourceAnalysis{
		acquisition: call,
		summaries:   resourceSummaries.Provider(pass),
		pass:        pass, evidence: evidence, function: call.Parent(), resource: resource, candidate: call.Pos(),
		contract: contract, optional: optionalAcquisition, actions: map[ssa.Instruction]resourceAction{},
		probe: probe, pool: pool,
	}
	setup := analysis.prepareResourceFlow()
	if setup.State == proofs.EvidenceUnknown {
		return unknownResourceLifetime(setup.Reason)
	}

	flow := analysis.proveResourceFlow(errorValue)
	if flow.state != proofs.EvidenceProven {
		return flow
	}
	// DB-prepared driver statements belong to pooled connections, whose
	// finalClose closes their open statements. This does not settle Rows,
	// Tx, or Conn obligations, nor claim DB.Close is identical to Stmt.Close.
	// https://go.dev/src/database/sql/sql.go (driverConn.finalClose, DB.prepareDC)
	// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/driver_test.go#L2809
	if sqlDatabaseCall(call.Common(), "Prepare", "PrepareContext") && lifecycle.ProveEnclosingCompletion(lifecycle.EnclosingCompletionRequest{
		Function: call.Parent(), Value: ssaflow.CallReceiver(call.Common()), Methods: []string{"Close"},
		Budget: analysis.budget(10000),
	}).Proven() {
		return acceptedResourceLifetime(resourceReasonParentCleanup)
	}
	if analysis.pool.Exhausted() {
		return unknownResourceLifetime(resourceReasonBudgetExhausted)
	}
	entryBudget := analysis.budget(releaseSearchBudget)
	if processExitReclaims(call, contract, entryBudget) {
		return acceptedResourceLifetime(resourceReasonProcessExitReclaims)
	}
	if entryBudget.Exhausted() {
		return unknownResourceLifetime(resourceReasonBudgetExhausted)
	}
	return flow
}

// emitAction traces a settled or unknown label. An instruction labelled none
// is not traced: every instruction after the acquisition would emit one, and
// the flow's return-path steps already show where the obligation stood.
func (analysis *resourceAnalysis) emitAction(instruction ssa.Instruction, action resourceAction, reason resourceLifetimeReason) {
	if action == actionNone || !analysis.probe.Enabled() {
		return
	}
	label, outcome := "settled", analysisTrace.OutcomeAccepted
	if action == actionUnknown {
		label, outcome = "unknown", analysisTrace.OutcomeUnknown
	}
	analysis.probe.Label(analysisTrace.Step{
		Reason:   reason.String(),
		Outcome:  outcome,
		Pos:      instruction.Pos(),
		Function: analysis.function.String(),
		Details:  map[string]string{"instruction": instruction.String(), "label": label},
	})
}

// localCollection finds the local slice the resource was appended to, and
// traces the decision: the collection, or the use of it that declined the
// model and left the append unknown.
func (analysis *resourceAnalysis) localCollection() *localCollection {
	decision := findLocalCollection(analysis.evidence, analysis.resource, analysis.contract.cleanup, analysis.budget(proofs.QueryBudget))
	if !analysis.probe.Enabled() || decision.collection == nil && decision.declinedAt == nil {
		return decision.collection
	}
	step := analysisTrace.Step{
		Reason: resourceReasonAppendedToLocalCollection.String(), Outcome: analysisTrace.OutcomeAccepted,
		Function: analysis.function.String(), Details: map[string]string{},
	}
	if decision.collection == nil {
		step.Reason, step.Outcome, step.Pos = resourceReasonCollectionUseUnknown.String(), analysisTrace.OutcomeUnknown, decision.declinedAt.Pos()
		step.Details["instruction"] = decision.declinedAt.String()
	} else {
		step.Pos = decision.collection.appends[0].Pos()
		step.Details["loops"] = strconv.Itoa(len(decision.collection.released))
	}
	analysis.probe.Evidence(step)
	return decision.collection
}

// traceCollectionReleased records that the path left a loop that released
// every element of the collection holding the resource.
func (analysis *resourceAnalysis) traceCollectionReleased(header, done *ssa.BasicBlock) {
	if !analysis.probe.Enabled() {
		return
	}
	test := header.Instrs[len(header.Instrs)-1]
	analysis.probe.Evidence(analysisTrace.Step{
		Reason: resourceReasonCollectionReleased.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: test.Pos(),
		Function: analysis.function.String(), Details: map[string]string{"loop": test.String(), "done": strconv.Itoa(done.Index)},
	})
}

// traceUncertainEdge records an edge that made the obligation unknown: one
// that re-tests a guard the path already took the other way, or the false
// edge of Rows.Next, which may or may not have closed the rows. It is traced
// as a label because, like an opaque instruction, it is where the proof gave
// up on this path.
func (analysis *resourceAnalysis) traceUncertainEdge(block, successor *ssa.BasicBlock, reason resourceLifetimeReason) {
	if !analysis.probe.Enabled() {
		return
	}
	branch := block.Instrs[len(block.Instrs)-1]
	analysis.probe.Label(analysisTrace.Step{
		Reason: reason.String(), Outcome: analysisTrace.OutcomeUnknown,
		Pos: branch.Pos(), Function: block.Parent().String(),
		Details: map[string]string{"branch": branch.String(), "successor": strconv.Itoa(successor.Index)},
	})
}

// processExitReclaims accepts a resource that program exit genuinely cleans
// up. A leak does harm when it accumulates or when its cleanup has an effect
// that exit would lose. An acquisition that runs at most once from main.main
// cannot accumulate, and every path that leaves main ends the process, which
// closes descriptors and connections. Only contracts whose cleanup merely
// reclaims qualify: a compressor's Close flushes buffered data, a transaction
// must commit, and an inferred owner's Close is not known to be free of such
// effects, so all of those are still reported.
func processExitReclaims(call *ssa.Call, contract resourceContract, budget *proofs.SearchBudget) bool {
	switch contract.family {
	case resourceFamilyOS, resourceFamilyHTTP:
	case resourceFamilySQL:
		if slices.Contains(contract.cleanup, "Commit") {
			return false
		}
	default:
		return false
	}
	// A complete unique private caller chain keeps this process-local policy
	// out of unconditional callee facts. Repeated/escaping helpers cannot
	// establish that their acquisitions happen only once before process exit.
	// https://github.com/boxesandglue/boxesandglue/blob/79509f4b6b0e2e7a1d0562139ab4d9946d4be080/helper/main.go#L10-L35
	return ssaflow.RunsOnceThroughPrivateEntryCallsWithin(call, budget)
}
