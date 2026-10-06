package goroutineownership

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Retained owners and paired streams supply identity witnesses to the existing
// instruction classifier. Their cleanup or handoff is unknown, never a join;
// the same return-flow proof decides whether those actions cover the worker.

// Observing cancellation of a context retained by an opaque worker argument
// makes its shutdown uncertain. The selected edge alone gets this credit;
// neither merely constructing a request nor another select arm is a join.
// https://github.com/zmap/zgrab2/blob/a1231792c51576f1818825fae51db042b4dcd41e/lib/http/server.go#L3649-L3705
func (analysis *spawnAnalysis) selectedOwnershipEdge(from, to *ssa.BasicBlock) bool {
	if analysis.selectedJoinEdge(from, to) {
		return true
	}
	channel, selected := ssaflow.SelectedReceiveOnEdge(from, to)
	if !selected {
		return false
	}
	proof := analysis.observesOpaqueWorkerContext(channel)
	if proof.State == proofs.EvidenceDisproven {
		return false
	}
	if proof.Reason == proofs.EvidenceBudgetExhausted {
		analysis.recordEdge(from, to, reasonRetainedOwnerBudgetExhausted, ssaflow.ObligationUnknown)
		return true
	}
	analysis.recordEdge(from, to, reasonSelectedContextEdge, ssaflow.ObligationUnknown)
	return true
}

func (analysis *spawnAnalysis) observesOpaqueWorkerContext(channel ssa.Value) proofs.Proof {
	budget := analysis.budget()
	found := analysis.observesOpaqueWorkerContextWithin(channel, budget)
	return analysis.retainedOwnerProof(found, budget)
}

func (analysis *spawnAnalysis) observesOpaqueWorkerContextWithin(channel ssa.Value, budget *proofs.SearchBudget) bool {
	call, ok := channel.(*ssa.Call)
	if !ok || !ssaflow.CallMatchesSymbol(call.Common(),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "context", Receiver: "Context", Name: "Done"})) {
		return false
	}
	function, closure := resolveSpawnedFunction(analysis.pass, analysis.spawn, budget)
	if function == nil || closure == nil {
		return false
	}
	publishes := workerHasSendWithin(function, budget) || workerHandsOffOutputChannelWithin(function, budget)
	if publishes || budget.Exhausted() {
		return false
	}
	retained := analysis.retainedWorkerOwner(ssaflow.CallReceiver(call.Common()), budget)
	evidence, _ := summaryKnowledge.Provider(analysis.pass).LifecycleEvidence("goroutineownership", string(check.GoroutineJoin))
	evidence.ForCandidate(analysis.spawn.Pos())
	for pair := range ssaflow.CallBindingsWithin(analysis.spawn.Common(), function, closure, budget) {
		if ssaflow.NewReachingWalk(carryForms).Within(budget).Any(pair.Supplied, retained) &&
			evidence.ClosureHandsValueToUnreadableCalleeWithin(closure, pair.Supplied, budget) {
			return true
		}
	}
	return false
}

// A captured reader may retain the connection the caller closes. Retention
// establishes possible lifecycle participation, not that closing joins the
// worker: the constructor may retain its argument somewhere besides its result.
// Requiring a positive summary avoids treating an ignored argument as an owner.
// https://github.com/abshkbh/arrakis/blob/877231496acbf3b3091ab33340d2d126a251c4d5/cmd/vsockclient/main.go#L30-L76
func (analysis *spawnAnalysis) closesRetainedWorkerOwner(instruction ssa.Instruction, common *ssa.CallCommon) proofs.Proof {
	budget := analysis.budget()
	found := analysis.closesRetainedWorkerOwnerWithin(instruction, common, budget)
	return analysis.retainedOwnerProof(found, budget)
}

// A cutoff leaves ownership uncertain at this call or selected edge. It does
// not excuse a return that flow reaches without that observation.
func (analysis *spawnAnalysis) retainedOwnerProof(found bool, budget *proofs.SearchBudget) proofs.Proof {
	if budget.Exhausted() {
		budget.Observe(proofs.EvidenceBudgetExhausted, analysis.spawn.Pos(), func() map[string]string {
			return map[string]string{"phase": "retained-owner"}
		})
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	if found {
		return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk}
	}
	return proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
}

func (analysis *spawnAnalysis) closesRetainedWorkerOwnerWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *proofs.SearchBudget,
) bool {
	receivers := cleanupTargets(common, budget)
	if len(receivers) == 0 {
		return false
	}
	if _, deferred := instruction.(*ssa.Defer); !deferred &&
		!slices.Contains(cfg.InstructionsReachableAfterWithin(analysis.spawn, budget), instruction) {
		return false
	}
	function, closure := resolveSpawnedFunction(analysis.pass, analysis.spawn, budget)
	if function == nil || workerHasSendWithin(function, budget) || budget.Exhausted() {
		return false
	}
	for _, receiver := range receivers {
		if !budget.Spend() {
			return false
		}
		retained := analysis.retainedWorkerOwner(receiver, budget)
		for pair := range ssaflow.CallBindingsWithin(analysis.spawn.Common(), function, closure, budget) {
			if ssaflow.NewReachingWalk(carryForms).Within(budget).Any(pair.Supplied, retained) {
				return true
			}
		}
		if analysis.opaqueWorkerUsesOwner(function, closure, receiver, budget) {
			return true
		}
	}
	return false
}

// A worker can consume a field of its captured owner rather than the capture
// itself. Match that projection through the shared identity query, without
// widening a wrapper's constructor arguments into ownership. Possible opaque
// use makes cleanup uncertain, never a proven worker join.
// https://github.com/lynxbase/lynxdb/blob/7c4bf0432b0cef2807f0ddcd2cd2000ce7ffb8c1/pkg/ingest/receiver/otlpgrpc/server.go#L106-L121
func (analysis *spawnAnalysis) opaqueWorkerUsesOwner(
	function *ssa.Function, closure *ssa.MakeClosure, receiver ssa.Value, budget *proofs.SearchBudget,
) bool {
	storage := heapmodel.NewStorage(budget)
	bindings := ssaflow.CallBindingsWithin(analysis.spawn.Common(), function, closure, budget)
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			continue
		}
		common := call.Common()
		callee, _ := ssaflow.DirectCallee(common)
		used := ssaflow.CallReceiver(common)
		if used == nil || callee != nil && len(callee.Blocks) != 0 || !opaqueCallEndsWorkerWork(call, budget) {
			continue
		}
		for pair := range bindings {
			if !ssaflow.ValueIsAccessPathFromWithin(receiver, pair.Supplied, budget) {
				continue
			}
			if cell, ok := pair.Supplied.(*ssa.Alloc); ok && !storage.StableContent(cell, analysis.spawn).Proven() {
				continue
			}
			if ssaflow.ProveIdentityWithin(
				ssaflow.AccessPath{Value: used, Root: pair.Local},
				ssaflow.AccessPath{Value: receiver, Root: pair.Supplied}, budget,
			).Proven() {
				return true
			}
		}
	}
	return false
}

// Cleanup of the opaque call's receiver cannot release later independent work.
// Admit only the worker's nonblocking completion tail, not a second call, send,
// receive, or loop. This narrows the new field mapping, not existing retention.
func opaqueCallEndsWorkerWork(call *ssa.Call, budget *proofs.SearchBudget) bool {
	for _, instruction := range cfg.InstructionsReachableAfterWithin(call, budget) {
		if !budget.Spend() || cfg.BlockInCycle(instruction.Block()) {
			return false
		}
		switch typed := instruction.(type) {
		case *ssa.Return, *ssa.Jump, *ssa.DebugRef:
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return false
			}
		case *ssa.Call:
			if !ssaflow.CallMatchesSymbol(typed.Common(), syntax.Builtin("close")) {
				return false
			}
		case *ssa.RunDefers:
			if !completionOnlyDefersWithin(call.Parent(), budget) {
				return false
			}
		default:
			return false
		}
	}
	return !budget.Exhausted()
}

func (analysis *spawnAnalysis) retainedWorkerOwner(receiver ssa.Value, budget *proofs.SearchBudget) func(ssaflow.ReachingWalk, ssa.Value) bool {
	evidence, _ := summaryKnowledge.Provider(analysis.pass).LifecycleEvidence("goroutineownership", string(check.GoroutineJoin))
	evidence.ForCandidate(analysis.spawn.Pos())
	storage := heapmodel.NewStorage(budget)
	identity := receiver
	// A nested worker captures an interface cell while its parent's deferred
	// close invokes the loaded interface. Peeling that load is identity only.
	// https://github.com/inguardians/peirates/blob/054aee1453b3e3d2779d2ebe0fd888fef8a4e224/internal/modules/dockersocket/dockersocket_linux_test.go#L215-L221
	if source, ok := ssaflow.IdentitySource(receiver); ok {
		identity = source
	}
	var retained func(ssaflow.ReachingWalk, ssa.Value) bool
	retained = func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		if heapmodel.MayAlias(value, identity) || heapmodel.CapturedBindingMatches(value, receiver) {
			return true
		}
		if content := storage.Resolve(value); content.Proven() && content.Value != value {
			return walk.Any(content.Value, retained)
		}
		switch typed := value.(type) {
		case *ssa.Alloc:
			content := storage.Content(typed, analysis.spawn)
			return content.Proven() && content.Value != value && walk.Any(content.Value, retained)
		case *ssa.Extract:
			return walk.Any(typed.Tuple, retained)
		case *ssa.Call:
			for index, argument := range typed.Common().Args {
				if !budget.Spend() {
					return false
				}
				if holds, _ := evidence.ArgumentRetained(typed, index); holds && walk.Any(argument, retained) {
					return true
				}
			}
		}
		return false
	}
	return retained
}

// io.Pipe and net.Pipe return communicating endpoints, not arbitrary sibling
// results. Passing the exact peer to another participant can release worker I/O.
// This only supplies unknown-use evidence after launch, never a completion claim.
// https://github.com/mutagen-io/mutagen/blob/6ccfeaaf4dfd261e59ef9aac56e3c157b62e605b/pkg/integration/protocols/netpipe/synchronization.go#L63-L95
func (analysis *spawnAnalysis) spawnedPipePeers(budget *proofs.SearchBudget) []trackedValue {
	function, closure := resolveSpawnedFunction(analysis.pass, analysis.spawn, budget)
	if function == nil || workerHasSendWithin(function, budget) {
		return nil
	}
	var peers []trackedValue
	storage := heapmodel.NewStorage(budget)
	var find func(ssaflow.ReachingWalk, ssa.Value) bool
	find = func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		// Resolve an endpoint at its own read and require stability for a
		// captured cell. This supplies only a possible peer witness; opaque
		// storage never proves the absence of a communicating owner.
		if content := storage.Resolve(value); content.Proven() && content.Value != value {
			return walk.Any(content.Value, find)
		}
		if cell, ok := value.(*ssa.Alloc); ok {
			content := storage.StableContent(cell, analysis.spawn)
			return content.Proven() && content.Value != value && walk.Any(content.Value, find)
		}
		result, ok := value.(*ssa.Extract)
		if !ok || result.Index > 1 {
			return false
		}
		call, ok := result.Tuple.(*ssa.Call)
		if !ok || !pipeConstructor(call.Common()) {
			return false
		}
		if peer := ssaflow.CallResultWithin(call, 1-result.Index, budget); peer != nil {
			peers = append(peers, trackedValue{value: peer, kind: trackedOwner})
			return true
		}
		return false
	}
	for pair := range ssaflow.CallBindingsWithin(analysis.spawn.Common(), function, closure, budget) {
		ssaflow.NewReachingWalk(carryForms).Within(budget).Any(pair.Supplied, find)
	}
	return peers
}

func pipeConstructor(common *ssa.CallCommon) bool {
	return ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("io", "Pipe")) ||
		ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("net", "Pipe"))
}

func (analysis *spawnAnalysis) pipePeerAction(instruction ssa.Instruction, common *ssa.CallCommon) ownershipAction {
	if len(analysis.pipePeers) == 0 {
		return actionNone
	}
	if _, deferred := instruction.(*ssa.Defer); !deferred &&
		!slices.Contains(cfg.InstructionsReachableAfter(analysis.spawn), instruction) {
		return actionNone
	}
	callee, closure := ssaflow.DirectCallee(common)
	_, launched := instruction.(*ssa.Go)
	if callee != nil && len(callee.Blocks) != 0 && !launched {
		if analysis.helperAction(common, callee, closure, analysis.pipePeers).action != actionNone {
			return actionUnknown
		}
		return actionNone
	}
	values := common.Args
	if closure != nil {
		values = append(slices.Clone(values), closure.Bindings...)
	}
	for _, peer := range analysis.pipePeers {
		if slices.ContainsFunc(values, func(value ssa.Value) bool {
			return carries(ssaflow.NewReachingWalk(carryForms), value, peer.value)
		}) {
			return actionUnknown
		}
	}
	return actionNone
}
