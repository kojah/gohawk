package goroutineownership

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
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
	if !selected || !analysis.observesOpaqueWorkerContext(channel) {
		return false
	}
	analysis.recordEdge(from, to, reasonSelectedContextEdge)
	return true
}

func (analysis *spawnAnalysis) observesOpaqueWorkerContext(channel ssa.Value) bool {
	call, ok := channel.(*ssa.Call)
	if analysis.config.mode != goroutineModeContext || !ok || !ssaflow.CallMatchesSymbol(call.Common(),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "context", Receiver: "Context", Name: "Done"})) {
		return false
	}
	function, closure := spawnedFunction(analysis.pass, analysis.spawn)
	if function == nil || closure == nil || workerHasSend(function) || workerHandsOffOutputChannel(function) {
		return false
	}
	retained := analysis.retainedWorkerOwner(ssaflow.CallReceiver(call.Common()))
	evidence, _ := summaryKnowledge.Provider(analysis.pass).LifecycleEvidence("goroutineownership", string(check.GoroutineJoin))
	evidence.ForCandidate(analysis.spawn.Pos())
	for _, pair := range ssaflow.CallBindings(analysis.spawn.Common(), function, closure) {
		if ssaflow.NewReachingWalk(carryForms).Any(pair.Supplied, retained) &&
			evidence.ClosureHandsValueToUnreadableCallee(closure, pair.Supplied) {
			return true
		}
	}
	return false
}

// An opaque helper receiving a send-capable channel can publish after its
// context is canceled. Capacity does not bound its send count. Badwolf's storage
// implementation ignores cancellation while streaming results, so the caller's
// canceled-context arm must not hide the abandoned producer behind the helper.
// https://github.com/google/badwolf/blob/6cde56dbc7db828597ea856db6c3e1f331e70916/storage/memoization/memoization.go#L194-L222
func workerHandsOffOutputChannel(function *ssa.Function) bool {
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		common := call.Common()
		if _, builtin := common.Value.(*ssa.Builtin); builtin {
			continue
		}
		if callee := common.StaticCallee(); callee != nil && len(callee.Blocks) > 0 {
			continue
		}
		for _, argument := range common.Args {
			channel, ok := argument.Type().Underlying().(*types.Chan)
			if ok && channel.Dir() != types.RecvOnly && !ssaflow.DefinitelyNil(argument) {
				return true
			}
		}
	}
	return false
}

// A captured reader may retain the connection the caller closes. Retention
// establishes possible lifecycle participation, not that closing joins the
// worker: the constructor may retain its argument somewhere besides its result.
// Requiring a positive summary avoids treating an ignored argument as an owner.
// https://github.com/abshkbh/arrakis/blob/877231496acbf3b3091ab33340d2d126a251c4d5/cmd/vsockclient/main.go#L30-L76
func (analysis *spawnAnalysis) closesRetainedWorkerOwner(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if analysis.config.mode == goroutineModeJoin {
		return false
	}
	receivers := cleanupTargets(common)
	if len(receivers) == 0 {
		return false
	}
	if _, deferred := instruction.(*ssa.Defer); !deferred &&
		!slices.Contains(ssaflow.InstructionsReachableAfter(analysis.spawn), instruction) {
		return false
	}
	function, closure := spawnedFunction(analysis.pass, analysis.spawn)
	if function == nil || workerHasSend(function) {
		return false
	}
	for _, receiver := range receivers {
		retained := analysis.retainedWorkerOwner(receiver)
		for _, pair := range ssaflow.CallBindings(analysis.spawn.Common(), function, closure) {
			if ssaflow.NewReachingWalk(carryForms).Any(pair.Supplied, retained) {
				return true
			}
		}
	}
	return false
}

// A factory's returned callback is cleanup evidence only when a returned
// literal captures the exact sibling resource and the existing helper-use
// proof establishes its lifecycle call. Arbitrary tuple siblings prove nothing.
// https://github.com/containerd/ttrpc/blob/9e62ff76048c2565d85b243f70adc66aeea73290/server_test.go#L571-L582
func cleanupTargets(common *ssa.CallCommon) []ssa.Value {
	if receiver := ssaflow.CallReceiver(common); lifecycleOwner(receiver) && lifecycleMethod(ssaflow.CallName(common)) {
		return []ssa.Value{receiver}
	}
	callback, ok := common.Value.(*ssa.Extract)
	if !ok {
		return nil
	}
	factory, ok := callback.Tuple.(*ssa.Call)
	if !ok {
		return nil
	}
	return factoryCleanupTargets(factory, callback.Index)
}

func factoryCleanupTargets(factory *ssa.Call, callbackIndex int) []ssa.Value {
	function := ssaflow.ResolvedCallee(factory.Common())
	if function == nil {
		return nil
	}
	var targets []ssa.Value
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	for index := range function.Signature.Results().Len() {
		target := ssaflow.CallResult(factory, index)
		if !lifecycleOwner(target) {
			continue
		}
		if lifecycle.ProveReturnedCleanup(function, lifecycle.ReturnedCleanupRelation{
			CallbackResult: callbackIndex, Target: index, TargetIsResult: true,
		}, lifecycle.CompletionRequest{Methods: []string{"Close", "Stop", "Shutdown"}, Budget: budget}).Proven() {
			targets = append(targets, target)
		}
	}
	// Preserve the older may-cleanup evidence below as Unknown only. A partial
	// literal return may explain shutdown participation, but cannot be promoted
	// to exact cleanup or a worker join. The shared relation above is stricter
	// and also understands forwarding factories.
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return targets
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			closure, ok := lifecycle.ReturnedResult(returned, callbackIndex).(*ssa.MakeClosure)
			if !ok {
				continue
			}
			for index := range returned.Results {
				target := ssaflow.CallResult(factory, index)
				if lifecycleOwner(target) && callbackClosesSibling(closure, lifecycle.ReturnedResult(returned, index), budget) {
					targets = append(targets, target)
				}
			}
		}
	}
	return targets
}

func callbackClosesSibling(closure *ssa.MakeClosure, sibling ssa.Value, budget *ssaflow.SearchBudget) bool {
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return false
	}
	for _, pair := range ssaflow.CallBindings(nil, function, closure) {
		if !budget.Spend() {
			return false
		}
		if !heapmodel.DefinitelySameValue(ssaflow.CapturedBindingValue(pair.Supplied), sibling) {
			continue
		}
		search := newHelperSearch()
		search.budget = budget
		if search.use(function, pair.Local, trackedOwner) == actionJoin {
			return true
		}
	}
	return false
}

func (analysis *spawnAnalysis) retainedWorkerOwner(receiver ssa.Value) func(ssaflow.ReachingWalk, ssa.Value) bool {
	evidence, _ := summaryKnowledge.Provider(analysis.pass).LifecycleEvidence("goroutineownership", string(check.GoroutineJoin))
	evidence.ForCandidate(analysis.spawn.Pos())
	budget := analysis.budget()
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
		if !budget.Spend() {
			return false
		}
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
				if holds, _ := evidence.ArgumentRetained(typed, index); holds && walk.Any(argument, retained) {
					return true
				}
			}
		}
		return false
	}
	return retained
}

// Releasing an I/O operation cannot settle a subsequent blocking publication.
// Keep the existing completion proof authoritative for workers with sends.
// https://github.com/pterodactyl/wings/blob/d6116827313dae176ddf4741e233554392993398/server/transfer/source.go#L87-L96
func workerHasSend(function *ssa.Function) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if _, send := instruction.(*ssa.Send); send {
				return true
			}
			if choice, ok := instruction.(*ssa.Select); ok && slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
				return state.Dir == types.SendOnly
			}) {
				return true
			}
		}
	}
	return false
}

// io.Pipe and net.Pipe return communicating endpoints, not arbitrary sibling
// results. Passing the exact peer to another participant can release worker I/O.
// This only supplies unknown-use evidence after launch, never a completion claim.
// https://github.com/mutagen-io/mutagen/blob/6ccfeaaf4dfd261e59ef9aac56e3c157b62e605b/pkg/integration/protocols/netpipe/synchronization.go#L63-L95
func (analysis *spawnAnalysis) spawnedPipePeers() []trackedValue {
	function, closure := spawnedFunction(analysis.pass, analysis.spawn)
	if function == nil || workerHasSend(function) {
		return nil
	}
	var peers []trackedValue
	budget := analysis.budget()
	storage := heapmodel.NewStorage(budget)
	var find func(ssaflow.ReachingWalk, ssa.Value) bool
	find = func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		if !budget.Spend() {
			return false
		}
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
		if peer := ssaflow.CallResult(call, 1-result.Index); peer != nil {
			peers = append(peers, trackedValue{value: peer, kind: trackedOwner})
			return true
		}
		return false
	}
	for _, pair := range ssaflow.CallBindings(analysis.spawn.Common(), function, closure) {
		ssaflow.NewReachingWalk(carryForms).Any(pair.Supplied, find)
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
		!slices.Contains(ssaflow.InstructionsReachableAfter(analysis.spawn), instruction) {
		return actionNone
	}
	callee, closure := ssaflow.DirectCallee(common)
	_, launched := instruction.(*ssa.Go)
	if callee != nil && len(callee.Blocks) != 0 && !launched {
		if analysis.helperAction(common, callee, closure, analysis.pipePeers) != actionNone {
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
