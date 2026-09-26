package producerlifecycle

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Workers left behind by a return. The count proof of abandoned-send asks
// whether a receive exists for every send; it cannot see a receive that is
// on some paths and not others, which is the classic goroutine leak: a
// worker sends its result on an unbuffered channel, and the function returns
// on a timeout or an error without receiving it. The reverse leaks too: a
// worker waits for a stop signal the function sends or closes on success but
// skips on an error return.
//
// The proof reports only when it has seen every use of the channel. The
// channel is made in the function, and the census of ssaflow.ChannelValues
// must end at operations in the function and in one goroutine it launches
// once; any other use, a channel handed on, kept, returned, or used by a
// second goroutine, declines. The worker performs exactly one operation on
// the channel, on every path, outside any select: a send, a receive, or a
// range, which ends only on a close. Then nothing but the function can
// complete it, and one flow query over the function's paths after the
// launch decides: a feasible normal return with no completing operation
// before it leaves the worker blocked forever. A send needs an unbuffered
// channel, since a buffer takes the value, and a function that also closes
// the channel declines, since a send on a closed channel panics rather than
// blocks.

type workerOperation uint8

const (
	workerSends workerOperation = iota + 1
	workerReceivesOnce
	workerRanges
)

type returnedWorker struct {
	spawn     *ssa.Go
	name      string
	operation workerOperation
	// completes are the function's instructions that complete the worker's
	// operation; receives also count on the select arm that receives.
	completes []ssa.Instruction
	channel   []ssa.Value
}

func reportReturnedWorkers(pass *analysis.Pass, function *ssa.Function) {
	provider := summaryKnowledge.Provider(pass)
	for _, made := range ssaflow.InstructionsOf[*ssa.MakeChan](function) {
		worker, reason := findReturnedWorker(function, made)
		if worker == nil {
			// A channel some goroutine uses that the proof declined says why,
			// so a recall census can count what the check cannot see.
			if reason != reasonNone {
				probe := trace.For(pass, "producerlifecycle", "", made.Pos())
				probe.Candidate(trace.Step{Reason: reasonLocalChannel.String(), Outcome: trace.OutcomeObserved})
				probe.Decision(trace.Step{Reason: reason.String(), Outcome: trace.OutcomeUnknown})
			}
			continue
		}
		id := check.ProducerLifecycleUnreceivedReturn
		if worker.operation != workerSends {
			id = check.ProducerLifecycleUnsignalledReceiver
		}
		probe := trace.For(pass, "producerlifecycle", string(id), worker.spawn.Pos())
		probe.Candidate(trace.Step{Reason: reason.String(), Outcome: trace.OutcomeObserved})
		if worker.completedBeforeLaunch() {
			probe.Decision(trace.Step{Reason: reasonCallerCompletesEveryReturn.String(), Outcome: trace.OutcomeAccepted})
			continue
		}
		outcome, witness := ssaflow.EvaluateObligationWitness(ssaflow.ObligationFlow{
			Start: worker.spawn, Instruction: worker.label, Edge: worker.edgeLabel,
			Successors: provider.Successors(), Terminates: provider.Terminates(),
		})
		if outcome != ssaflow.ObligationViolated || witness == nil {
			probe.Decision(trace.Step{Reason: reasonCallerCompletesEveryReturn.String(), Outcome: trace.OutcomeAccepted})
			continue
		}
		probe.Decision(trace.Step{Reason: reasonReturnWithoutCounterpart.String(), Outcome: trace.OutcomeRejected, Pos: witness.Pos()})
		reportWorker(pass, id, worker, witness)
	}
}

func reportWorker(pass *analysis.Pass, id check.ID, worker *returnedWorker, witness *ssa.Return) {
	message := "goroutine blocks forever sending on " + worker.name + ": the function can return without receiving"
	relatedMessage := "returns here without receiving"
	switch worker.operation {
	case workerReceivesOnce:
		message = "goroutine blocks forever receiving from " + worker.name + ": the function can return without sending on or closing it"
		relatedMessage = "returns here without sending on or closing " + worker.name
	case workerRanges:
		message = "goroutine blocks forever receiving from " + worker.name + ": the function can return without closing it"
		relatedMessage = "returns here without closing " + worker.name
	case workerSends:
	}
	source := syntax.SourceRange(pass, worker.spawn.Pos())
	diagnostic := analysis.Diagnostic{Pos: source.Pos(), End: source.End(), Message: message}
	if at := returnPosition(witness); at.IsValid() {
		returned := syntax.SourceRange(pass, at)
		diagnostic.Related = []analysis.RelatedInformation{{Pos: returned.Pos(), End: returned.End(), Message: relatedMessage}}
	}
	check.Report(pass, id, diagnostic)
}

// returnPosition cites a return statement, or the closing brace of the
// function for the implicit return at its end, which has no position.
func returnPosition(returned *ssa.Return) token.Pos {
	if returned.Pos().IsValid() {
		return returned.Pos()
	}
	switch syntax := returned.Parent().Syntax().(type) {
	case *ast.FuncDecl:
		if syntax.Body != nil {
			return syntax.Body.Rbrace
		}
	case *ast.FuncLit:
		return syntax.Body.Rbrace
	}
	return token.NoPos
}

// findReturnedWorker takes the census of a channel made in function and
// returns the one-shot worker it serves, or nil with the reason it declined.
func findReturnedWorker(function *ssa.Function, made *ssa.MakeChan) (*returnedWorker, producerReason) {
	// The census: every use of the channel must be an operation, and every
	// operation must be in the function or in the one function it launches.
	// Anything else means someone the proof cannot see may complete the
	// worker's operation.
	values, uses := ssaflow.ChannelValues(made)
	var callerOps, workerOps []ssa.Instruction
	var worker *ssa.Function
	for _, use := range uses {
		if !channelOperation(use) {
			return nil, reasonChannelEscapes
		}
		owner := use.Instruction.Parent()
		switch {
		case owner == function:
			callerOps = append(callerOps, use.Instruction)
		case worker == nil || owner == worker:
			worker = owner
			workerOps = append(workerOps, use.Instruction)
		default:
			return nil, reasonChannelEscapes
		}
	}
	if worker == nil {
		return nil, reasonNone
	}
	// The worker must be launched once, by one go statement, and do one
	// thing with the channel on every path; then its fate depends only on
	// what the function does after the launch.
	spawn, reason := soleLaunch(function, worker, values)
	if spawn == nil {
		return nil, reason
	}
	operation, reason := oneShotOperation(worker, workerOps)
	if operation == 0 {
		return nil, reason
	}
	result := &returnedWorker{spawn: spawn, name: channelName(made, worker, values), operation: operation, channel: values}
	// A buffer takes the worker's one value, so only an unbuffered send can
	// block for want of a receiver.
	if operation == workerSends && !unbuffered(made) {
		return nil, reasonChannelBuffered
	}
	// The function's own operations must fit the worker's; those that
	// complete it become the flow query's exact actions.
	for _, op := range callerOps {
		allowed, completes := result.callerOperation(op)
		if !allowed {
			return nil, reasonCallerOperationsMixed
		}
		if completes {
			result.completes = append(result.completes, op)
		}
	}
	return result, reasonOneShotWorker
}

// channelOperation reports whether a use is an operation on the channel the
// proof understands, or one that does not affect it, such as len.
func channelOperation(use ssaflow.ChannelUse) bool {
	switch typed := use.Instruction.(type) {
	case *ssa.Send, *ssa.Select, *ssa.DebugRef:
		return true
	case *ssa.UnOp:
		return typed.Op == token.ARROW
	case *ssa.Call, *ssa.Defer:
		builtin, ok := ssaflow.InstructionCall(typed).Value.(*ssa.Builtin)
		return ok && (builtin.Name() == "close" || builtin.Name() == "len" || builtin.Name() == "cap")
	}
	return false
}

// soleLaunch returns the go statement that launches worker, provided it is
// the only way the channel reaches the worker and it runs at most once per
// call of function.
func soleLaunch(function *ssa.Function, worker *ssa.Function, values []ssa.Value) (*ssa.Go, producerReason) {
	var launch *ssa.Go
	for _, spawn := range ssaflow.InstructionsOf[*ssa.Go](function) {
		if launchedFunction(spawn.Common()) == worker {
			if launch != nil {
				return nil, reasonWorkerLaunchedRepeatedly
			}
			launch = spawn
		}
	}
	if launch == nil {
		return nil, reasonChannelEscapes
	}
	if ssaflow.BlockInCycle(launch.Block()) {
		return nil, reasonWorkerLaunchedRepeatedly
	}
	if closure, ok := launch.Call.Value.(*ssa.MakeClosure); ok {
		if len(*closure.Referrers()) != 1 {
			return nil, reasonChannelEscapes
		}
		return launch, reasonNone
	}
	// A named worker receives the channel as an argument; no other call in
	// the function may hand it the channel.
	for _, value := range values {
		if value.Parent() != function || value.Referrers() == nil {
			continue
		}
		for _, user := range *value.Referrers() {
			common := ssaflow.InstructionCall(user)
			if common != nil && common.StaticCallee() == worker && user != ssa.Instruction(launch) {
				return nil, reasonChannelEscapes
			}
		}
	}
	return launch, reasonNone
}

// oneShotOperation classifies the worker's only operation on the channel:
// a send or a single receive on every path and outside any loop, or a range,
// a comma-ok receive in a loop, which ends only on a close.
func oneShotOperation(worker *ssa.Function, ops []ssa.Instruction) (workerOperation, producerReason) {
	if len(ops) != 1 || !reachedOnEveryReturn(worker, ops[0]) {
		return 0, reasonWorkerOperationUnsupported
	}
	inLoop := ssaflow.BlockInCycle(ops[0].Block())
	switch typed := ops[0].(type) {
	case *ssa.Send:
		if !inLoop {
			return workerSends, reasonNone
		}
	case *ssa.UnOp:
		switch {
		case typed.Op != token.ARROW:
		case !inLoop:
			return workerReceivesOnce, reasonNone
		case typed.CommaOk:
			return workerRanges, reasonNone
		}
	}
	return 0, reasonWorkerOperationUnsupported
}

func reachedOnEveryReturn(worker *ssa.Function, op ssa.Instruction) bool {
	returns := ssaflow.InstructionsOf[*ssa.Return](worker)
	return len(returns) != 0 && !slices.ContainsFunc(returns, func(returned *ssa.Return) bool {
		return !op.Block().Dominates(returned.Block())
	})
}

func unbuffered(made *ssa.MakeChan) bool {
	size, ok := made.Size.(*ssa.Const)
	if !ok || size.Value == nil || size.Value.Kind() != constant.Int {
		return false
	}
	capacity, exact := constant.Int64Val(size.Value)
	return exact && capacity == 0
}

// callerOperation reports whether the function's operation fits the
// worker's, and whether it completes it: a receive completes a send, a send
// or a close completes a single receive, and only a close completes a range,
// which the function feeds with sends. An operation that does not fit, such
// as a close while the worker sends, which panics it, declines the proof.
func (worker *returnedWorker) callerOperation(op ssa.Instruction) (allowed, completes bool) {
	switch typed := op.(type) {
	case *ssa.DebugRef:
		return true, false
	case *ssa.Select:
		sendsOnChannel := slices.ContainsFunc(typed.States, func(state *ssa.SelectState) bool {
			return worker.carries(state.Chan) && state.Dir != types.RecvOnly
		})
		// The arm that receives completes a send; edgeLabel marks it.
		return worker.operation == workerSends && !sendsOnChannel, false
	case *ssa.UnOp:
		return worker.operation == workerSends, true
	case *ssa.Send:
		return worker.operation != workerSends, worker.operation == workerReceivesOnce
	}
	switch builtinName(op) {
	case "close":
		return worker.operation != workerSends, true
	case "len", "cap":
		return true, false
	}
	return false, false
}

func builtinName(op ssa.Instruction) string {
	common := ssaflow.InstructionCall(op)
	if common == nil {
		return ""
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return builtin.Name()
	}
	return ""
}

func (worker *returnedWorker) carries(value ssa.Value) bool {
	return slices.Contains(worker.channel, value)
}

// label marks an instruction that completes the worker's operation. A
// select completes it only on the arm that receives, which edgeLabel marks.
func (worker *returnedWorker) label(instruction ssa.Instruction) ssaflow.ObligationAction {
	if slices.Contains(worker.completes, instruction) {
		return ssaflow.ObligationExact
	}
	return ssaflow.ObligationNone
}

func (worker *returnedWorker) edgeLabel(from, to *ssa.BasicBlock) ssaflow.ObligationAction {
	if worker.operation != workerSends {
		return ssaflow.ObligationNone
	}
	if channel, ok := ssaflow.SelectedReceiveOnEdge(from, to); ok && worker.carries(channel) {
		return ssaflow.ObligationExact
	}
	return ssaflow.ObligationNone
}

// completedBeforeLaunch reports whether a deferred close registered before
// the launch completes the worker on every return: the walk starts after the
// launch and would not see it.
func (worker *returnedWorker) completedBeforeLaunch() bool {
	return worker.operation != workerSends && slices.ContainsFunc(worker.completes, func(op ssa.Instruction) bool {
		_, deferred := op.(*ssa.Defer)
		return deferred && ssaflow.InstructionDominates(op, worker.spawn)
	})
}

// channelName names the channel as the source does: the variable a closure
// captures, or the worker's parameter.
func channelName(made *ssa.MakeChan, worker *ssa.Function, values []ssa.Value) string {
	for _, user := range *made.Referrers() {
		if store, ok := user.(*ssa.Store); ok {
			if cell, ok := store.Addr.(*ssa.Alloc); ok && cell.Comment != "" {
				return cell.Comment
			}
		}
	}
	for _, value := range values {
		if parameter, ok := value.(*ssa.Parameter); ok && parameter.Parent() == worker {
			return parameter.Name()
		}
	}
	return "its channel"
}
