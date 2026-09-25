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
	"github.com/kojah/gohawk/internal/resourcemodel"
	"github.com/kojah/gohawk/internal/ssaflow"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Resource flow tracks an acquired value from the successful call edge to each
// feasible normal return. State records activation and release separately so
// error-only resources and path-specific cleanup do not create false leaks.

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
	block       int
	predecessor int
	index       int
	obligation  resourcemodel.Obligation
	guards      string
}

// Analyzer returns this package's configured Go analysis pass.

func evaluateResourceFlow(
	pass *analysis.Pass,
	evidence *lifecyclefacts.LifecycleEvidence,
	call *ssa.Call,
	resource ssa.Value,
	contract resourceContract,
) resourceLifetimePolicyResult {
	index := ssaflow.InstructionIndex(call)
	if index < 0 {
		return acceptedResourceLifetime(resourceReasonReleaseProven)
	}
	errorValue := acquisitionErrorResult(call)
	if reason := httpAcquisitionBoundary(pass, call); reason != resourceReasonNone {
		return acceptedResourceLifetime(reason)
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
	if deferredBeforeAcquisitionMayRelease(evidence, call, resource, contract.cleanup) {
		return acceptedResourceLifetime(resourceReasonReleaseProven)
	}
	owners := localResourceOwners(call.Parent(), resource)
	analysis := &resourceAnalysis{
		acquisition: call,
		summaries:   resourceSummaries.Provider(pass),
		pass:        pass, evidence: evidence, function: call.Parent(), resource: resource, candidate: call.Pos(), owners: owners,
		contract: contract, optional: optionalAcquisition, actions: map[ssa.Instruction]resourceAction{},
		probe: analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos()),
	}
	if analysis.cleanupRegisteredBefore(call) {
		return acceptedResourceLifetime(resourceReasonOpaqueConsumption)
	}
	if !analysis.acquisitionReachable() {
		return acceptedResourceLifetime(resourceReasonAcquisitionUnreachable)
	}
	// The walk starts on the instruction after the acquisition and keys its
	// states by block, predecessor, and release status, so the same block is
	// revisited only when a different path reaches it with a different
	// obligation state; the predecessor lets the successful branch of the
	// acquisition be told apart from its error branch.
	initial := []resourceFlowState{{block: call.Block(), index: index + 1, obligation: resourcemodel.Acquired(), guards: ssaflow.GuardsDominating(call)}}
	opaque, leaks := false, false
	ssaflow.WalkStates(initial, resourceStateKey, func(state resourceFlowState) ([]resourceFlowState, bool) {
		state, leaks = advanceResourceState(analysis, state)
		if leaks {
			return nil, false
		}
		opaque = opaque || state.obligation.Unknown()
		return resourceSuccessorStates(analysis, state, errorValue), true
	})
	if leaks {
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
		return reportedResourceLifetime(resourceReasonUnownedReturn)
	}
	if opaque {
		return acceptedResourceLifetime(resourceReasonOpaqueConsumption)
	}
	return acceptedResourceLifetime(resourceReasonReleaseProven)
}

func resourceStateKey(state resourceFlowState) resourceFlowKey {
	predecessor := -1
	if state.predecessor != nil {
		predecessor = state.predecessor.Index
	}
	return resourceFlowKey{
		block:       state.block.Index,
		predecessor: predecessor,
		index:       state.index,
		obligation:  state.obligation,
		guards:      state.guards.Key(),
	}
}

func advanceResourceState(analysis *resourceAnalysis, state resourceFlowState) (resourceFlowState, bool) {
	// A release or transfer anywhere before a return settles the path. An
	// opaque consumption does not settle it but removes the proof: the
	// return is then neither owned nor a defect.
	for _, instruction := range state.block.Instrs[state.index:] {
		state.guards = state.guards.After(instruction)
		switch analysis.action(instruction) {
		case actionSettled:
			state.obligation = state.obligation.Discharged()
		case actionUnknown:
			state.obligation = state.obligation.Uncertain()
		case actionNone:
		}
		// A call that never returns, whether os.Exit or a project's own fatal
		// wrapper the summaries prove, ends this path with nothing to release.
		if ssaflow.InstructionTerminatesWith(instruction, analysis.summaries.Terminates()) {
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
		if ok && state.obligation.Unsettled() &&
			!analysis.returnedResourceOwner(returned) &&
			!heapmodel.ReturnedMayAliasAny(returned, analysis.owners) {
			return state, true
		}
	}
	return state, false
}

func resourceSuccessorStates(analysis *resourceAnalysis, state resourceFlowState, errorValue ssa.Value) []resourceFlowState {
	pass, resource, optionalAcquisition, candidate := analysis.pass, analysis.resource, analysis.optional, analysis.candidate
	successors := analysis.feasibleSuccessors(state)
	if optionalAcquisition.Proven() && state.block == optionalAcquisition.merge && state.predecessor == optionalAcquisition.acquisitionBlock {
		successors = []*ssa.BasicBlock{optionalAcquisition.acquiredSuccessor}
		traceOptionalAcquisition(pass, optionalAcquisition, candidate)
	}
	result := make([]resourceFlowState, 0, len(successors))
	for _, successor := range successors {
		obligation := state.obligation
		if success, known := resourceSuccessBranch(pass, analysis.summaries, state.block, successor, errorValue, candidate); known {
			if !success {
				obligation = obligation.Absent()
			}
		}
		if present, known := resourcePresenceBranch(state.block, successor, resource); known {
			if !present {
				obligation = obligation.Absent()
			}
		}
		guards, contradiction := state.guards.Extend(state.block, successor, nil)
		contradicted := contradiction != ssaflow.GuardConsistent
		if contradicted {
			analysis.traceRepeatedGuard(state.block, successor)
		}
		if sqlRowsExhaustionEdge(state.block, successor, resource) || contradicted {
			obligation = obligation.Uncertain()
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
		result = append(result, resourceFlowState{
			block: successor, predecessor: state.block, obligation: obligation, guards: guards,
		})
	}
	return result
}

// traceRepeatedGuard records that an edge re-tested a guard the path had
// already taken the other way, so the path became unknown there.
func (analysis *resourceAnalysis) traceRepeatedGuard(block, successor *ssa.BasicBlock) {
	if !analysis.probe.Enabled() {
		return
	}
	branch := block.Instrs[len(block.Instrs)-1]
	analysis.probe.Evidence(analysisTrace.Step{
		Reason: resourceReasonRepeatedGuardEdgeUnknown.String(), Outcome: analysisTrace.OutcomeUnknown,
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
	for _, result := range returned.Results {
		if !heapmodel.ValueDerivesFrom(result, resource, map[ssa.Value]bool{}) {
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
					if heapmodel.ValueDerivesFrom(argument, errorValue, map[ssa.Value]bool{}) {
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

func resourcePresenceBranch(block, successor *ssa.BasicBlock, resource ssa.Value) (bool, bool) {
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
	if asserted := assertedResource(branch.Cond, resource); asserted {
		return successor == block.Succs[0], true
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	comparesResourceToNil := heapmodel.ValueDerivesFrom(comparison.X, resource, map[ssa.Value]bool{}) && ssaflow.DefinitelyNil(comparison.Y) ||
		heapmodel.ValueDerivesFrom(comparison.Y, resource, map[ssa.Value]bool{}) && ssaflow.DefinitelyNil(comparison.X)
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
		heapmodel.ValueDerivesFrom(assertion.X, resource, map[ssa.Value]bool{})
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
		if releaseSettled(evidence.Prove(lifecyclefacts.EvidenceRequest{Instruction: deferred, Target: resource, Completion: &completion})) {
			return true
		}
	}
	return false
}
