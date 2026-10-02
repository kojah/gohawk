package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"

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
	errorValue := acquisitionErrorResult(call)
	if reason := httpAcquisitionBoundary(pass, call); reason != resourceReasonNone {
		return unknownResourceLifetime(reason)
	}
	if acquisitionContextCanceled(call) {
		return acceptedResourceLifetime(resourceReasonCanceledAcquisition)
	}
	if testProvesAcquisitionError(call, resource, errorValue, contract.packagePath == "net/http") {
		return acceptedResourceLifetime(resourceReasonReleaseProven)
	}
	optionalAcquisition := proveOptionalAcquisition(call, resource, errorValue)
	if optionalAcquisition.Proven() {
		resource = optionalAcquisition.resourcePhi
	}
	// This pre-acquisition query uses anywhere coverage: a deferred loop may
	// release the resource, but it does not prove every-return settlement of
	// this exact acquisition. Preserve that uncertainty in the final proof.
	if deferredBeforeAcquisitionMayRelease(evidence, call, resource, contract.cleanup) {
		return unknownResourceLifetime(resourceReasonPriorDeferMayRelease)
	}
	owners := localResourceOwners(call.Parent(), resource)
	analysis := &resourceAnalysis{
		acquisition: call,
		summaries:   resourceSummaries.Provider(pass),
		pass:        pass, evidence: evidence, function: call.Parent(), resource: resource, candidate: call.Pos(), owners: owners,
		contract: contract, optional: optionalAcquisition, actions: map[ssa.Instruction]resourceAction{},
		probe: analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos()),
	}
	analysis.collection = analysis.localCollection()
	analysis.guardedDefers = analysis.findResultGuardedDefers()
	if analysis.cleanupRegisteredBefore(call) {
		return unknownResourceLifetime(resourceReasonOpaqueConsumption)
	}
	flow := analysis.proveResourceFlow(errorValue)
	if flow.state != ssaflow.EvidenceProven {
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
	if processExitReclaims(call, contract) {
		return acceptedResourceLifetime(resourceReasonProcessExitReclaims)
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
	decision := findLocalCollection(analysis.evidence, analysis.resource, analysis.contract.cleanup, analysis.budget(ssaflow.QueryBudget))
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

// returnedResourceOwner reports whether the return hands the resource to the
// caller through a value that can still release it. When a result derives
// from the resource but is declined, the trace says which rule declined it:
// a summarized view, or a projection with no cleanup method.
func (analysis *resourceAnalysis) returnedResourceOwner(returned *ssa.Return) bool {
	resource, cleanup := analysis.resource, analysis.contract.cleanup
	if lifecycle.ReturnedValueOwnsValue(returned, resource) {
		return true
	}
	if position := analysis.returnedWrapperPosition(returned); position >= 0 &&
		analysis.evidence.RetainingResultClaimed(analysis.function, position) {
		analysis.traceReturnedResult(returned, returned.Results[position], resourceReasonReturnedRetainingWrapper, analysisTrace.OutcomeAccepted)
		return true
	}
	for _, result := range returned.Results {
		if !heapmodel.ValueDerivesFrom(result, resource) {
			continue
		}
		// Narrowing an interface preserves its dynamic object, including Close:
		// the caller can still recover io.Closer by assertion. Require the
		// unchanged cleanup-bearing projection, not a transformed reader or a
		// replacement body that merely occupies the original field.
		// https://github.com/lich0821/ccNexus/blob/55887d232555f94ea4db621a5a7e65430eebf0d7/internal/transformer/tool_chain.go#L121-L135
		if original, changed := ssaflow.UnwrapTransparentValue(result, ssaflow.TransparentChangeInterface); changed &&
			heapmodel.NewStorage(analysis.budget(1000)).Projection(original, resource, returned).Proven() {
			result = original
		}
		// A returned view is summarized as releasing nothing, whatever its
		// method names suggest; the caller of this function cannot close the
		// resource through it.
		if call, ok := result.(*ssa.Call); ok && resourceSummaries.Provider(analysis.pass).CallReturnsView(call, resource) {
			analysis.traceReturnedResult(returned, result, resourceReasonReturnedViewCannotRelease, analysisTrace.OutcomeRejected)
			continue
		}
		methods := types.NewMethodSet(result.Type())
		for method := range methods.Methods() {
			if slices.Contains(cleanup, method.Obj().Name()) {
				analysis.traceReturnedResult(returned, result, resourceReasonReturnedCleanupProjection, analysisTrace.OutcomeAccepted)
				return true
			}
		}
		analysis.traceReturnedResult(returned, result, resourceReasonReturnedProjectionLacksCleanup, analysisTrace.OutcomeRejected)
	}
	return false
}

func (analysis *resourceAnalysis) traceReturnedResult(returned *ssa.Return, result ssa.Value, reason resourceLifetimeReason, outcome analysisTrace.Outcome) {
	if !analysis.probe.Enabled() {
		return
	}
	step := analysisTrace.Step{
		Reason: reason.String(), Outcome: outcome, Pos: returned.Pos(), Function: returned.Parent().String(),
		Details: map[string]string{"result": result.Name(), "result_type": result.Type().String()},
	}
	if outcome == analysisTrace.OutcomeAccepted {
		analysis.probe.Evidence(step)
		return
	}
	analysis.probe.Considered(step)
}

func testProvesAcquisitionError(acquisition *ssa.Call, resource, errorValue ssa.Value, httpResponse bool) bool {
	// Test assertions can prove the owned-resource path infeasible even though
	// the assertion package expresses that fact outside the CFG.
	// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/cmd/wfx/cmd/root/root_test.go#L154-L157
	errorAssertions, nilAssertions := httpErrorAssertions(acquisition, resource, errorValue)
	// A fatal Error assertion stops the test unless the acquisition failed,
	// which is the same evidence as an `if err != nil { return }` guard for any
	// acquisition. The non-fatal form is accepted only for net/http, whose
	// paired Nil assertion carries the extra fact that a response returned
	// together with an error has an already-closed body.
	for _, assertedError := range errorAssertions {
		if fatalErrorAssertion(assertedError) || httpResponse && errorAssertionDominatesNil(assertedError, nilAssertions) {
			return true
		}
	}
	return false
}

func httpErrorAssertions(acquisition *ssa.Call, resource, errorValue ssa.Value) ([]ssa.Instruction, []ssa.Instruction) {
	var errorAssertions, nilAssertions []ssa.Instruction
	for _, block := range acquisition.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !ssaflow.InstructionMayFollow(acquisition, instruction) {
				continue
			}
			common := ssaflow.InstructionCall(instruction)
			if ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyErrorClaim) {
				for _, argument := range common.Args {
					if heapmodel.ValueDerivesFrom(argument, errorValue) {
						errorAssertions = append(errorAssertions, instruction)
					}
				}
			}
			if ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyNilClaim) {
				for _, argument := range common.Args {
					if heapmodel.MayAlias(argument, resource) {
						nilAssertions = append(nilAssertions, instruction)
					}
				}
			}
		}
	}
	return errorAssertions, nilAssertions
}

func errorAssertionDominatesNil(assertedError ssa.Instruction, nilAssertions []ssa.Instruction) bool {
	for _, assertedNil := range nilAssertions {
		if ssaflow.InstructionDominates(assertedError, assertedNil) {
			return true
		}
	}
	return false
}

func fatalErrorAssertion(instruction ssa.Instruction) bool {
	common := ssaflow.InstructionCall(instruction)
	return ssaflow.HasLibraryContract(common, ssaflow.ContractTestifyFatalError)
}

// holdsResource reports whether a compared value may be the resource: it
// derives from the resource and its type can hold it. An error returned by a
// helper that was handed the file derives from the file, but no error value
// is the file, so its nil check says nothing about whether the file exists.
func holdsResource(value, resource ssa.Value) bool {
	return types.AssignableTo(resource.Type(), value.Type()) && heapmodel.ValueDerivesFrom(value, resource)
}

// presenceOperand reports whether a nil comparison of value decides whether
// the resource holds anything to release: value is the resource itself, or
// the Body of the net/http response that is the resource. A response whose
// Body is nil has nothing to close, and net/http documents that a response
// returned without error always has a non-nil Body, so a close guarded by
// `resp != nil && resp.Body != nil` covers every feasible path:
// https://github.com/Authula/authula/blob/87a880a2872fae95d749d7a250db0274524fafce/plugins/oauth2/services/base_provider.go#L63-L74
func presenceOperand(value, resource ssa.Value) bool {
	if holdsResource(value, resource) {
		return true
	}
	field := lifecyclefacts.ResponseBodyField(value)
	return field != nil && holdsResource(field.X, resource)
}

func resourcePresenceBranch(block, predecessor, successor *ssa.BasicBlock, resource ssa.Value) (bool, bool) {
	if resource == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	// A comma-ok assertion of the resource's own static type, or of an
	// interface it implements, holds when the asserted value is the
	// resource: the false arm has no owned value to release. moby keeps an
	// io.Reader that may be a file and defers Close under the assertion:
	// https://github.com/moby/moby/blob/3f6733064ea2ea9c00a4a2a9c5c9c5fbd7b7b1d5/daemon/builder/remotecontext/internal/tarsum/tarsum_test.go#L347-L349
	// Saving a short-circuit condition introduces a phi. Only the incoming
	// comparison on this path supplies evidence: an unrelated flag or a phi
	// from an earlier block cannot establish that this resource is absent.
	condition := ssaflow.BranchValue(branch.Cond, block, predecessor)
	if asserted := assertedResource(condition, resource); asserted {
		return successor == block.Succs[0], true
	}
	comparison, ok := condition.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	comparesResourceToNil := presenceOperand(comparison.X, resource) && ssaflow.DefinitelyNil(comparison.Y) ||
		presenceOperand(comparison.Y, resource) && ssaflow.DefinitelyNil(comparison.X)
	if !comparesResourceToNil {
		return false, false
	}
	trueBranch := successor == block.Succs[0]
	// On the nil branch there is no owned value to release. This matters when
	// callers defensively close a response whenever net/http returns one, even
	// on an error path:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/codex_utls_transport_test.go#L305-L319
	if comparison.Op == token.NEQ {
		return trueBranch, true
	}
	return !trueBranch, true
}

// assertedResource reports whether condition is the ok result of a comma-ok
// type assertion whose operand resolves to the resource and whose asserted
// type the resource's static type satisfies, so the assertion succeeds.
func assertedResource(condition, resource ssa.Value) bool {
	okResult, ok := condition.(*ssa.Extract)
	if !ok || okResult.Index != 1 {
		return false
	}
	assertion, ok := okResult.Tuple.(*ssa.TypeAssert)
	if !ok || !assertion.CommaOk {
		return false
	}
	// The asserted operand is usually a load of the cell the resource was
	// stored into, which other paths may have written too; possible
	// derivation suffices, because the rule only ever removes an
	// obligation from the arm where the assertion failed.
	held := heapmodel.NewStorage(nil).Same(assertion.X, resource).Proven() ||
		heapmodel.ValueDerivesFrom(assertion.X, resource)
	return held && types.AssignableTo(resource.Type(), assertion.AssertedType)
}

// deferredBeforeAcquisitionMayRelease reports whether a defer registered on
// every path to the acquisition may release the resource: typically a literal
// that drains a captured closer slice the resource is appended to later. The
// walk below only classifies instructions after the acquisition, so such a
// defer is asked here, with may-release coverage because the deferred
// literal decides at return time how many entries it closes. rules_img opens
// inputs into a closer slice under one deferred drain loop:
// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/mtree/mtree.go#L110-L128
func deferredBeforeAcquisitionMayRelease(
	evidence *lifecyclefacts.LifecycleEvidence,
	call *ssa.Call,
	resource ssa.Value,
	methods []string,
) bool {
	for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](call.Parent()) {
		if !ssaflow.InstructionDominates(deferred, call) {
			continue
		}
		completion := lifecycle.CompletionRequest{
			Instruction: deferred,
			Target:      resource,
			Methods:     methods,
			Coverage:    lifecycle.CoverageAnywhere,
			Budget:      ssaflow.NewSearchBudget(releaseSearchBudget),
		}
		proof := evidence.Prove(lifecyclefacts.EvidenceRequest{Instruction: deferred, Target: resource, Completion: &completion})
		// This may-release boundary keeps an exhausted search uncertain. It
		// does not turn that early exit into an exact instruction discharge.
		action, reason := releaseLabel(proof)
		if action == actionSettled || reason == resourceReasonBudgetExhausted {
			return true
		}
	}
	return false
}

// processExitReclaims accepts a resource that program exit genuinely cleans
// up. A leak does harm when it accumulates or when its cleanup has an effect
// that exit would lose. An acquisition that runs at most once in main.main
// cannot accumulate, and every path that leaves main ends the process, which
// closes descriptors and connections. Only contracts whose cleanup merely
// reclaims qualify: a compressor's Close flushes buffered data, a transaction
// must commit, and an inferred owner's Close is not known to be free of such
// effects, so all of those are still reported.
func processExitReclaims(call *ssa.Call, contract resourceContract) bool {
	switch contract.family {
	case "os", "http":
	case "sql":
		if slices.Contains(contract.cleanup, "Commit") {
			return false
		}
	default:
		return false
	}
	return ssaflow.RunsOnceInProgramEntry(call)
}
