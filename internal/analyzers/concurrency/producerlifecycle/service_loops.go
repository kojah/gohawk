package producerlifecycle

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A service loop is a goroutine that serves a struct-owned channel from a
// select inside a loop, and returns when a stop arm of that select fires.
// After it returns, a plain send on the served channel has no receiver and
// blocks forever. The proof needs the closed-world channel inventory, receivers
// that are all stoppable background loops, a stop signal that can fire, and a
// sender that is an entry point with no escape and no lifecycle guard. Each
// failed requirement is a stable reason and leaves the send unreported.
// Real-world shape: gocronx-team/cron and ErnestK/MCPSprut
// (benchmarks/precision/audits/deadlock-fixes-2026-09-24.md).

type loopReason uint8

const (
	loopReasonNone loopReason = iota
	loopReasonCandidate
	loopReasonChannelEscapes
	loopReasonChannelSupplied
	loopReasonChannelBuffered
	loopReasonChannelCopied
	loopReasonPlainReceive
	loopReasonChannelClosed
	loopReasonNoReceiver
	loopReasonReceiverNotBackground
	loopReasonReceiveNotInLoop
	loopReasonNoStopArm
	loopReasonStopUnsignalled
	loopReasonSenderInLoop
	loopReasonSenderLaunchesLoop
	loopReasonSenderNotEntryPoint
	loopReasonGuardProtects
	loopReasonGuardUnknown
	loopReasonStoppedLoopSend
	loopReasonCount
)

var loopReasonCodes = [...]string{
	loopReasonNone:                  "",
	loopReasonCandidate:             "owned-channel-send",
	loopReasonChannelEscapes:        "owned-channel-escapes",
	loopReasonChannelSupplied:       "owned-channel-supplied",
	loopReasonChannelBuffered:       "owned-channel-buffered",
	loopReasonChannelCopied:         "owned-channel-copied",
	loopReasonPlainReceive:          "owned-channel-plain-receive",
	loopReasonChannelClosed:         "owned-channel-closed",
	loopReasonNoReceiver:            "owned-channel-no-receiver",
	loopReasonReceiverNotBackground: "receiver-not-background-loop",
	loopReasonReceiveNotInLoop:      "receive-not-in-loop",
	loopReasonNoStopArm:             "service-loop-no-stop-arm",
	loopReasonStopUnsignalled:       "service-loop-stop-unsignalled",
	loopReasonSenderInLoop:          "sender-in-service-loop",
	loopReasonSenderLaunchesLoop:    "sender-launches-service-loop",
	loopReasonSenderNotEntryPoint:   "sender-not-entry-point",
	loopReasonGuardProtects:         "send-guarded-by-locked-state",
	loopReasonGuardUnknown:          "send-guard-unknown",
	loopReasonStoppedLoopSend:       "send-after-service-loop-stops",
}

func (reason loopReason) String() string {
	if int(reason) >= len(loopReasonCodes) {
		return "invalid-service-loop-reason"
	}
	return loopReasonCodes[reason]
}

var contextDone = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "context", Receiver: "Context", Name: "Done"})

var uncancellableContexts = []syntax.Symbol{
	syntax.PackageFunction("context", "Background"), syntax.PackageFunction("context", "TODO"),
}

type loopProof struct {
	reason loopReason
	stop   token.Pos
	// stops are the stop-arm channels of every serving loop that can fire.
	stops []ssa.Value
}

func (proof loopProof) proven() bool { return proof.reason == loopReasonStoppedLoopSend }

// reportStoppedLoopSends visits each plain send on an owned channel in an
// analyzed function. The inventory is built once, and only if a candidate
// exists, because it scans the whole package.
func reportStoppedLoopSends(pass *analysis.Pass, functions []*ssa.Function) {
	var inventory *channelInventory
	for _, function := range functions {
		for _, send := range ssaflow.InstructionsOf[*ssa.Send](function) {
			field := sentField(pass.Pkg, send)
			if field == nil {
				continue
			}
			if inventory == nil {
				inventory = newChannelInventory(pass)
			}
			probe := trace.For(pass, "producerlifecycle", string(check.ProducerLifecycleStoppedLoop), send.Pos())
			probe.Candidate(trace.Step{Reason: loopReasonCandidate.String(), Outcome: trace.OutcomeObserved})
			proof := proveStoppedLoopSend(inventory, field, send)
			outcome := trace.OutcomeUnknown
			if proof.proven() {
				outcome = trace.OutcomeRejected
			}
			probe.Decision(trace.Step{Reason: proof.reason.String(), Outcome: outcome})
			if proof.proven() {
				source := syntax.SourceRange(pass, send.Pos())
				check.Report(pass, check.ProducerLifecycleStoppedLoop, analysis.Diagnostic{
					Pos: source.Pos(), End: source.End(), Message: "send can block forever after the service loop receiving it returns",
					Related: []analysis.RelatedInformation{{Pos: proof.stop, Message: "service loop returns when this select takes its stop arm"}},
				})
			}
		}
	}
}

// sentField returns the owned field a plain send's channel was loaded from.
func sentField(pkg *types.Package, send *ssa.Send) *types.Var {
	load, ok := send.Chan.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	address, ok := load.X.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	return ownedChannelField(pkg, address)
}

// proveStoppedLoopSend is the one decision for this check: channel evidence,
// then every receiver, then the send itself.
func proveStoppedLoopSend(inventory *channelInventory, field *types.Var, send *ssa.Send) loopProof {
	owned := inventory.fields[field]
	switch {
	case owned == nil:
		return loopProof{reason: loopReasonChannelEscapes}
	case owned.unknown != loopReasonNone:
		return loopProof{reason: owned.unknown}
	case owned.closed:
		// A send after close panics rather than blocks; that is another defect.
		return loopProof{reason: loopReasonChannelClosed}
	case len(owned.receives) == 0:
		// With no receiver at all, the defect is not a stopped loop.
		return loopProof{reason: loopReasonNoReceiver}
	}
	var stop token.Pos
	var stops []ssa.Value
	loops := map[*ssa.Function]bool{}
	for _, selected := range owned.receives {
		proof := proveServiceLoop(inventory, selected, owned)
		if proof.reason != loopReasonNone {
			return proof
		}
		loops[selected.Parent()] = true
		stops = append(stops, proof.stops...)
		if !stop.IsValid() {
			stop = proof.stop
		}
	}
	if reason := unguardedEntrySend(inventory, send, loops, stops); reason != loopReasonNone {
		return loopProof{reason: reason}
	}
	return loopProof{reason: loopReasonStoppedLoopSend, stop: stop}
}

// proveServiceLoop requires a receive that is a select arm inside a loop of a
// function started only by go, and another receive arm of the same select
// whose branch returns without re-entering the select.
func proveServiceLoop(inventory *channelInventory, selected *ssa.Select, served *ownedChannel) loopProof {
	function := selected.Parent()
	if !inventory.launches.startedOnlyByGo(function) {
		return loopProof{reason: loopReasonReceiverNotBackground}
	}
	if !ssaflow.BlockInCycle(selected.Block()) {
		return loopProof{reason: loopReasonReceiveNotInLoop}
	}
	unsignalled := false
	var stops []ssa.Value
	for _, state := range selected.States {
		if state.Dir != types.RecvOnly || servesField(state.Chan, served) {
			continue
		}
		arm := selectArmBlock(selected, state.Chan)
		if arm == nil || !returnsAvoiding(arm, selected.Block()) {
			continue
		}
		if !stopCanFire(inventory, state.Chan, function) {
			unsignalled = true
			continue
		}
		stops = append(stops, state.Chan)
	}
	switch {
	case len(stops) != 0:
		return loopProof{stop: selected.Pos(), stops: stops}
	case unsignalled:
		return loopProof{reason: loopReasonStopUnsignalled}
	default:
		return loopProof{reason: loopReasonNoStopArm}
	}
}

func servesField(channel ssa.Value, served *ownedChannel) bool {
	load, ok := channel.(*ssa.UnOp)
	if !ok {
		return false
	}
	address, ok := load.X.(*ssa.FieldAddr)
	return ok && ownedChannelField(served.field.Pkg(), address) == served.field
}

// selectArmBlock returns the block entered only when the select takes the
// receive arm on channel.
func selectArmBlock(selected *ssa.Select, channel ssa.Value) *ssa.BasicBlock {
	for _, block := range selected.Parent().Blocks {
		if received, ok := ssaflow.SelectedReceiveChannel(block); ok && received == channel {
			return block
		}
	}
	return nil
}

// returnsAvoiding reports whether a normal return is reachable from start
// without passing through avoid, the block holding the loop's select.
func returnsAvoiding(start, avoid *ssa.BasicBlock) bool {
	seen := map[*ssa.BasicBlock]bool{avoid: true}
	queue := []*ssa.BasicBlock{start}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		if len(block.Instrs) != 0 {
			if _, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Return); ok {
				return true
			}
		}
		queue = append(queue, block.Succs...)
	}
	return false
}

// A stop arm can fire when it receives from an owned channel that the package
// sends on or closes, or from Done of a context the loop was given, unless
// every launch passes an uncancellable Background or TODO context.
func stopCanFire(inventory *channelInventory, channel ssa.Value, loop *ssa.Function) bool {
	if load, ok := channel.(*ssa.UnOp); ok && load.Op == token.MUL {
		address, ok := load.X.(*ssa.FieldAddr)
		if !ok {
			return false
		}
		field := ownedChannelField(loop.Pkg.Pkg, address)
		owned := inventory.fields[field]
		return field != nil && owned != nil && owned.signalled
	}
	done, ok := channel.(*ssa.Call)
	if !ok || !ssaflow.CallMatchesSymbol(done.Common(), contextDone) {
		return false
	}
	parameter, ok := ssaflow.CallReceiver(done.Common()).(*ssa.Parameter)
	if !ok {
		return false
	}
	index := parameterIndex(loop, parameter)
	for _, launchers := range inventory.launches.launchers {
		for _, launch := range launchers {
			if launchedFunction(launch.Common()) != loop || index >= len(launch.Call.Args) {
				continue
			}
			if call, ok := launch.Call.Args[index].(*ssa.Call); ok && ssaflow.CallMatchesAnySymbol(call.Common(), uncancellableContexts...) {
				return false
			}
		}
	}
	return index >= 0
}

func parameterIndex(function *ssa.Function, parameter *ssa.Parameter) int {
	for index, candidate := range function.Params {
		if candidate == parameter {
			return index
		}
	}
	return -1
}

// unguardedEntrySend checks the sender. Only an exported function or method is
// reported: an unexported helper is usually reached through a guarded entry
// point that this check does not follow. A branch on the owner's own state
// before the send is judged by proveLifecycleGuard.
func unguardedEntrySend(inventory *channelInventory, send *ssa.Send, loops map[*ssa.Function]bool, stops []ssa.Value) loopReason {
	function := send.Parent()
	switch {
	case loops[function]:
		return loopReasonSenderInLoop
	case launchesAny(inventory, function, loops):
		return loopReasonSenderLaunchesLoop
	case function.Parent() != nil || function.Object() == nil || !ast.IsExported(function.Object().Name()):
		return loopReasonSenderNotEntryPoint
	}
	owner := send.Chan.(*ssa.UnOp).X.(*ssa.FieldAddr).X
	switch proveLifecycleGuard(inventory, send, owner, stops) {
	case guardProtects:
		return loopReasonGuardProtects
	case guardUnknown:
		return loopReasonGuardUnknown
	default:
		// No guard, or one that races with the loop stopping.
		return loopReasonNone
	}
}

func launchesAny(inventory *channelInventory, function *ssa.Function, loops map[*ssa.Function]bool) bool {
	for _, launch := range inventory.launches.launchers[function] {
		if loops[launchedFunction(launch.Common())] {
			return true
		}
	}
	return false
}

// readsOwner reports whether a branch condition, within a few operations,
// reads the owner object: one of its fields, or a method call on it. The depth
// bound keeps this a structural test of the condition expression, not a flow
// analysis; phis are not followed.
func readsOwner(value ssa.Value, owner ssa.Value, depth int) bool {
	if depth == 0 || value == nil {
		return false
	}
	if value == owner {
		return true
	}
	if address, ok := value.(*ssa.FieldAddr); ok && address.X == owner {
		return true
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	if _, phi := instruction.(*ssa.Phi); phi {
		return false
	}
	for _, operand := range instruction.Operands(nil) {
		if readsOwner(*operand, owner, depth-1) {
			return true
		}
	}
	return false
}
