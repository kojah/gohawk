package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// This file owns the counted select drain, the one join that rests on a loop
// count. A receive completes only against a send, so receives can never
// outnumber sends. When a loop runs a blocking, receive-only select exactly N
// times, and the select's channels are made here with at most one send each
// per invocation, leaving the loop means at least N sends happened. If N
// covers every channel, every send happened, and each worker has passed its
// completion send. The proof counts messages rather than paths: it never
// unrolls the loop and never asks which arm ran.
//
// The proof abstains whenever it cannot list every sender. A channel stored
// anywhere else, handed to an unknown call, closed, or sent on inside a loop
// or by a worker launched more than once leaves the edge unproven, and the
// ordinary flow query decides as before.

// The count is compared, never unrolled, so the limit only keeps the header
// literal within a plausible fan-out.
const maxDrainIterations = 64

// countedDrainEdge reports whether leaving a counted loop proves that every
// channel of its select, including one of this worker's signals, was drained.
// go-quests drains five single-send workers this way:
// https://github.com/lite-quests/go-quests/blob/792cb31674bd8349b1c0407208823a51f39692ea/solutions/solution-017.select_timeout/select_timeout.go#L8-L55
func (analysis *spawnAnalysis) countedDrainEdge(from, to *ssa.BasicBlock) bool {
	if len(analysis.signals) == 0 {
		return false
	}
	loop := ssaflow.ProveCountedRegion(from, maxDrainIterations, analysis.budget())
	if !loop.Proven() || loop.Exit != to {
		return false
	}
	choice := blockingReceiveSelect(loop.Body)
	if choice == nil {
		return false
	}
	var channels []*ssa.MakeChan
	joined := false
	for _, state := range choice.States {
		made := analysis.singleSendChannel(state.Chan, choice)
		if made == nil {
			return false
		}
		if !slices.Contains(channels, made) {
			channels = append(channels, made)
		}
		joined = joined || analysis.isSignal(state.Chan)
	}
	if !joined || loop.Count < len(channels) {
		return false
	}
	analysis.recordEdge(from, to, reasonCountedDrainEdge)
	return true
}

// blockingReceiveSelect returns the select that the loop body's entry block
// runs once per iteration. A default arm or a send arm lets an iteration
// finish without consuming a message, so either one voids the count.
func blockingReceiveSelect(body *ssa.BasicBlock) *ssa.Select {
	for _, instruction := range body.Instrs {
		choice, ok := instruction.(*ssa.Select)
		if !ok {
			continue
		}
		receivesOnly := !slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
			return state.Dir != types.RecvOnly
		})
		if choice.Blocking && receivesOnly {
			return choice
		}
		return nil
	}
	return nil
}

// singleSendChannel resolves a select operand to a channel made once in this
// function and proves that at most one send on it can happen per invocation.
// The operand is either the channel itself, passed to its worker as an
// argument, or a load of the local cell that closures capture.
func (analysis *spawnAnalysis) singleSendChannel(operand ssa.Value, choice *ssa.Select) *ssa.MakeChan {
	var cell *ssa.Alloc
	var made *ssa.MakeChan
	switch typed := operand.(type) {
	case *ssa.MakeChan:
		made = typed
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return nil
		}
		cell, _ = typed.X.(*ssa.Alloc)
		made = singleStoredChannel(cell)
	}
	if made == nil || made.Parent() != analysis.function || ssaflow.BlockInCycle(made.Block()) {
		return nil
	}
	if cell != nil && (cell.Parent() != analysis.function || ssaflow.BlockInCycle(cell.Block())) {
		return nil
	}
	counter := sendCounter{choice: choice}
	counter.countChannelUses(made, cell)
	if cell != nil {
		counter.countCellUses(cell, made)
	}
	if !counter.exact || counter.sends > 1 {
		return nil
	}
	return made
}

// singleStoredChannel returns the channel stored into cell when that is its
// only store; a second store could swap the channel between iterations.
func singleStoredChannel(cell *ssa.Alloc) *ssa.MakeChan {
	if cell == nil || cell.Referrers() == nil {
		return nil
	}
	var made *ssa.MakeChan
	for _, use := range *cell.Referrers() {
		store, ok := use.(*ssa.Store)
		if !ok || store.Addr != cell {
			continue
		}
		stored, ok := store.Val.(*ssa.MakeChan)
		if !ok || made != nil {
			return nil
		}
		made = stored
	}
	return made
}

// sendCounter enumerates every use of one channel. exact stays true only
// while each use is a known receive, the single store into its cell, or a
// worker launched once whose only use of the channel is a send.
type sendCounter struct {
	choice *ssa.Select
	sends  int
	exact  bool
}

func (counter *sendCounter) countChannelUses(made *ssa.MakeChan, cell *ssa.Alloc) {
	counter.exact = true
	for _, use := range *made.Referrers() {
		switch typed := use.(type) {
		case *ssa.DebugRef:
		case *ssa.Store:
			counter.exact = counter.exact && cell != nil && typed.Addr == cell
		case *ssa.Select:
			counter.exact = counter.exact && typed == counter.choice
		case *ssa.Go:
			counter.countLaunchedSends(typed, made)
		case *ssa.ChangeType:
			// Passing the channel to a send-only parameter narrows its type
			// without copying it; the conversion may only feed a launch.
			counter.countNarrowedLaunches(typed)
		default:
			counter.exact = false
		}
	}
}

func (counter *sendCounter) countNarrowedLaunches(narrowed *ssa.ChangeType) {
	for _, use := range *narrowed.Referrers() {
		if _, debug := use.(*ssa.DebugRef); debug {
			continue
		}
		launch, ok := use.(*ssa.Go)
		if !ok {
			counter.exact = false
			return
		}
		counter.countLaunchedSends(launch, narrowed)
	}
}

func (counter *sendCounter) countCellUses(cell *ssa.Alloc, made *ssa.MakeChan) {
	for _, use := range *cell.Referrers() {
		switch typed := use.(type) {
		case *ssa.DebugRef:
		case *ssa.Store:
			counter.exact = counter.exact && typed.Val == made
		case *ssa.UnOp:
			counter.exact = counter.exact && typed.Op == token.MUL && onlyUsedBy(typed, counter.choice)
		case *ssa.MakeClosure:
			counter.countClosureSends(typed, cell)
		default:
			counter.exact = false
		}
	}
}

// countLaunchedSends counts the sends of a statically known worker that
// receives the channel as an argument. A launch inside a loop could start
// the worker several times, each with its own send.
func (counter *sendCounter) countLaunchedSends(launch *ssa.Go, channel ssa.Value) {
	callee := launch.Call.StaticCallee()
	if launch.Call.IsInvoke() || callee == nil || len(callee.Blocks) == 0 || ssaflow.BlockInCycle(launch.Block()) {
		counter.exact = false
		return
	}
	for index, argument := range launch.Call.Args {
		if argument == channel && index < len(callee.Params) {
			counter.countWorkerSends(callee.Params[index], false)
		}
	}
}

// countClosureSends counts the sends of a closure that captures the cell and
// is launched exactly once. Any other use of the closure could run it again.
func (counter *sendCounter) countClosureSends(closure *ssa.MakeClosure, cell *ssa.Alloc) {
	worker, ok := closure.Fn.(*ssa.Function)
	if !ok || closure.Referrers() == nil {
		counter.exact = false
		return
	}
	launches := 0
	for _, use := range *closure.Referrers() {
		launch, ok := use.(*ssa.Go)
		if _, debug := use.(*ssa.DebugRef); debug {
			continue
		}
		if !ok || launch.Call.Value != closure || ssaflow.BlockInCycle(launch.Block()) {
			counter.exact = false
			return
		}
		launches++
	}
	counter.exact = counter.exact && launches == 1
	for index, binding := range closure.Bindings {
		if binding == cell && index < len(worker.FreeVars) {
			counter.countWorkerSends(worker.FreeVars[index], true)
		}
	}
}

// countWorkerSends accepts only sends on the worker's copy of the channel. A
// captured cell is first loaded; a parameter is the channel itself.
func (counter *sendCounter) countWorkerSends(value ssa.Value, captured bool) {
	for _, use := range *value.Referrers() {
		if _, debug := use.(*ssa.DebugRef); debug {
			continue
		}
		if !captured {
			counter.countSend(use, value)
			continue
		}
		load, ok := use.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			counter.exact = false
			return
		}
		for _, loaded := range *load.Referrers() {
			if _, debug := loaded.(*ssa.DebugRef); !debug {
				counter.countSend(loaded, load)
			}
		}
	}
}

func (counter *sendCounter) countSend(use ssa.Instruction, channel ssa.Value) {
	send, ok := use.(*ssa.Send)
	if !ok || send.Chan != channel || send.X == channel || ssaflow.BlockInCycle(send.Block()) {
		counter.exact = false
		return
	}
	counter.sends++
}

func onlyUsedBy(value ssa.Value, want ssa.Instruction) bool {
	for _, use := range *value.Referrers() {
		if _, debug := use.(*ssa.DebugRef); !debug && use != want {
			return false
		}
	}
	return true
}
