package producerlifecycle

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A range over a channel ends only when the channel is closed. When one
// method closes an owned channel on its success path but returns an error
// without closing it, a goroutine ranging over that channel while the method
// runs waits forever after a failure. This file proves every link of that
// chain from the package's closed world:
//
//   - the channel is an owned field (see owned_channels.go), made in the
//     package, and never handed to other code;
//   - the range has no other way out: no break, return, or goto leaves it;
//   - exactly one function closes the field, on its own receiver, with no
//     deferred close;
//   - that function returns an error, and some return reached without the
//     close yields an error proven non-nil, so the skipped close is a failure
//     path rather than a step that a later call completes;
//   - the function is never called inside a loop, where a retry could close
//     the channel later;
//   - one function launches a goroutine that calls it on an object and,
//     concurrently, ranges over the same object's channel, in itself after
//     the launch or in another goroutine it launches. Both reach the object
//     through a captured variable written once.
//
// Anything else is unknown. A range with a select escape, a closer reached
// through an interface or a function value, and a producer in another
// package are out of scope.
// Real-world form: fiatjaf/nak's ThirdPartyNegentropy.Run closed Deltas only
// on success while a sibling goroutine ranged over it,
// https://github.com/fiatjaf/nak/blob/72e6e735f373d80909c80fe52a42d10c18215aa2/sync.go#L206-L209

type rangeReason uint8

const (
	rangeReasonNone rangeReason = iota
	rangeReasonCandidate
	rangeReasonChannelUnknown
	rangeReasonOtherExit
	rangeReasonNeverClosed
	rangeReasonSeveralClosers
	rangeReasonDeferredClose
	rangeReasonClosesOtherObject
	rangeReasonNoErrorResult
	rangeReasonClosedOnEveryReturn
	rangeReasonErrorUnproven
	rangeReasonCloserRepeated
	rangeReasonNotConcurrent
	rangeReasonUnclosedRange
	rangeReasonCount
)

var rangeReasonCodes = [...]string{
	rangeReasonNone:                "",
	rangeReasonCandidate:           "owned-channel-range",
	rangeReasonChannelUnknown:      "owned-channel-unknown",
	rangeReasonOtherExit:           "range-has-other-exit",
	rangeReasonNeverClosed:         "channel-never-closed",
	rangeReasonSeveralClosers:      "several-closing-functions",
	rangeReasonDeferredClose:       "close-deferred",
	rangeReasonClosesOtherObject:   "close-not-on-receiver",
	rangeReasonNoErrorResult:       "closer-returns-no-error",
	rangeReasonClosedOnEveryReturn: "closed-on-every-return",
	rangeReasonErrorUnproven:       "unclosed-return-error-unproven",
	rangeReasonCloserRepeated:      "closer-called-in-loop",
	rangeReasonNotConcurrent:       "closer-not-concurrent-with-range",
	rangeReasonUnclosedRange:       "range-waits-on-failed-closer",
}

var _ = [1]struct{}{}[len(rangeReasonCodes)-int(rangeReasonCount)]

func (reason rangeReason) String() string { return rangeReasonCodes[reason] }

type rangeProof struct {
	reason   rangeReason
	closer   *ssa.Function
	unclosed token.Pos
}

var (
	waitGroupGo  = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Go"})
	errorsNew    = syntax.PackageFunction("errors", "New")
	formatErrorf = syntax.PackageFunction("fmt", "Errorf")
	contextCause = syntax.PackageFunction("context", "Cause")
	contextErr   = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "context", Receiver: "Context", Name: "Err"})
)

// reportUnclosedRanges visits each receive from an owned channel field.
func reportUnclosedRanges(pass *analysis.Pass, functions []*ssa.Function) {
	var inventory *channelInventory
	var packageFunctions []*ssa.Function
	for _, function := range functions {
		for _, receive := range ssaflow.InstructionsOf[*ssa.UnOp](function) {
			field := receivedField(pass.Pkg, receive)
			if field == nil {
				continue
			}
			if inventory == nil {
				inventory = newChannelInventory(pass)
				packageFunctions = ssaflow.PackageFunctions(pass)
			}
			probe := trace.For(pass, "producerlifecycle", string(check.ProducerLifecycleUnclosed), receive.Pos())
			probe.Candidate(trace.Step{Reason: rangeReasonCandidate.String(), Outcome: trace.OutcomeObserved})
			proof := proveUnclosedRange(inventory, packageFunctions, field, receive)
			outcome := trace.OutcomeUnknown
			if proof.reason == rangeReasonUnclosedRange {
				outcome = trace.OutcomeRejected
			}
			probe.Decision(trace.Step{Reason: proof.reason.String(), Outcome: outcome})
			if proof.reason != rangeReasonUnclosedRange {
				continue
			}
			source := syntax.SourceRange(pass, receive.Pos())
			check.Report(pass, check.ProducerLifecycleUnclosed, analysis.Diagnostic{
				Pos: source.Pos(), End: source.End(),
				Message: "range can wait forever: " + proof.closer.Name() + " returns an error without closing the channel",
				Related: []analysis.RelatedInformation{{Pos: proof.unclosed, Message: "this return skips the close"}},
			})
		}
	}
}

// receivedField returns the owned field a receive's channel was loaded from.
func receivedField(pkg *types.Package, receive *ssa.UnOp) *types.Var {
	if receive.Op != token.ARROW || !receive.CommaOk {
		return nil
	}
	load, ok := receive.X.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	address, ok := load.X.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	return ownedChannelField(pkg, address)
}

// proveUnclosedRange is the one decision for this check, taking the links in
// the order the file comment lists them.
func proveUnclosedRange(inventory *channelInventory, functions []*ssa.Function, field *types.Var, receive *ssa.UnOp) rangeProof {
	owned := inventory.fields[field]
	if owned == nil || owned.unknown != loopReasonNone {
		return rangeProof{reason: rangeReasonChannelUnknown}
	}
	if !rangeWaitsForClose(receive) {
		return rangeProof{reason: rangeReasonOtherExit}
	}
	closer, reason := soleCloser(owned)
	if reason != rangeReasonNone {
		return rangeProof{reason: reason}
	}
	unclosed, reason := failedReturnSkipsClose(closer, owned.closes)
	if reason != rangeReasonNone {
		return rangeProof{reason: reason}
	}
	if reason := concurrentWithRange(functions, closer, receive); reason != rangeReasonNone {
		return rangeProof{reason: reason}
	}
	return rangeProof{reason: rangeReasonUnclosedRange, closer: closer, unclosed: unclosed}
}

// rangeWaitsForClose reports whether the receive drives a loop whose only
// normal exit is the receive reporting a closed channel. An exit into a block
// that panics, such as a range-over-func iterator's misuse check, crashes the
// program rather than ending the wait.
func rangeWaitsForClose(receive *ssa.UnOp) bool {
	header := receive.Block()
	loop, ok := ssaflow.NaturalLoopAt(header, ssaflow.NewSearchBudget(ssaflow.QueryBudget))
	if !ok || len(header.Succs) != 2 || len(header.Instrs) == 0 {
		return false
	}
	branch, ok := header.Instrs[len(header.Instrs)-1].(*ssa.If)
	if !ok {
		return false
	}
	received, ok := branch.Cond.(*ssa.Extract)
	closed := header.Succs[1]
	if !ok || received.Tuple != receive || received.Index != 1 || loop.Contains(closed) {
		return false
	}
	for _, exit := range loop.Exits {
		if exit != closed && !panicking(exit) {
			return false
		}
	}
	for _, predecessor := range closed.Preds {
		if predecessor != header && loop.Contains(predecessor) {
			return false
		}
	}
	return true
}

func panicking(block *ssa.BasicBlock) bool {
	if len(block.Instrs) == 0 {
		return false
	}
	_, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Panic)
	return ok
}

// soleCloser returns the one function that closes the field, when every close
// is a plain call on that function's receiver.
func soleCloser(owned *ownedChannel) (*ssa.Function, rangeReason) {
	var closer *ssa.Function
	for _, closing := range owned.closes {
		if _, deferred := closing.(*ssa.Defer); deferred {
			return nil, rangeReasonDeferredClose
		}
		if closer != nil && closing.Parent() != closer {
			return nil, rangeReasonSeveralClosers
		}
		closer = closing.Parent()
		if !closesReceiverField(closing, closer) {
			return nil, rangeReasonClosesOtherObject
		}
	}
	if closer == nil {
		return nil, rangeReasonNeverClosed
	}
	return closer, rangeReasonNone
}

func closesReceiverField(closing ssa.CallInstruction, closer *ssa.Function) bool {
	arguments := closing.Common().Args
	if len(arguments) != 1 || closer.Signature.Recv() == nil || len(closer.Params) == 0 {
		return false
	}
	load, ok := arguments[0].(*ssa.UnOp)
	if !ok {
		return false
	}
	address, ok := load.X.(*ssa.FieldAddr)
	return ok && address.X == closer.Params[0]
}

// failedReturnSkipsClose finds the returns the closer can reach without
// passing a close. It proves a failure path only when each of them returns a
// non-nil error.
func failedReturnSkipsClose(closer *ssa.Function, closes []ssa.CallInstruction) (token.Pos, rangeReason) {
	results := closer.Signature.Results()
	errorType := types.Universe.Lookup("error").Type()
	if results.Len() == 0 || !types.Identical(results.At(results.Len()-1).Type(), errorType) {
		return token.NoPos, rangeReasonNoErrorResult
	}
	closing := map[*ssa.BasicBlock]bool{}
	for _, call := range closes {
		closing[call.Block()] = true
	}
	var unclosed []*ssa.Return
	seen := map[*ssa.BasicBlock]bool{closer.Blocks[0]: true}
	for work := []*ssa.BasicBlock{closer.Blocks[0]}; len(work) != 0; {
		block := work[len(work)-1]
		work = work[:len(work)-1]
		if closing[block] {
			continue
		}
		if returned, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Return); ok {
			unclosed = append(unclosed, returned)
		}
		for _, next := range block.Succs {
			if !seen[next] {
				seen[next] = true
				work = append(work, next)
			}
		}
	}
	if len(unclosed) == 0 {
		return token.NoPos, rangeReasonClosedOnEveryReturn
	}
	for _, returned := range unclosed {
		if !nonNilError(returned.Results[len(returned.Results)-1], returned) {
			return token.NoPos, rangeReasonErrorUnproven
		}
	}
	return unclosed[0].Pos(), rangeReasonNone
}

// nonNilError reports whether an error value is non-nil where it is returned:
// a boxed concrete value, a result of errors.New or fmt.Errorf, both of which
// always return a non-nil error, a context's Err or Cause once its Done
// channel has fired, or a value the path has tested against nil.
func nonNilError(value ssa.Value, at *ssa.Return) bool {
	switch value := value.(type) {
	case *ssa.MakeInterface:
		return true
	case *ssa.Call:
		common := value.Common()
		if ssaflow.CallMatchesAnySymbol(common, errorsNew, formatErrorf) {
			return true
		}
		if ssaflow.CallMatchesSymbol(common, contextCause) && len(common.Args) == 1 {
			return afterDone(common.Args[0], at.Block())
		}
		if ssaflow.CallMatchesSymbol(common, contextErr) {
			return afterDone(ssaflow.CallReceiver(common), at.Block())
		}
	}
	return testedNonNil(value, at.Block())
}

// afterDone reports whether block runs only after a select arm received from
// ctx.Done(). The context package documents that once Done is closed, Err
// and Cause return a non-nil error.
func afterDone(ctx ssa.Value, block *ssa.BasicBlock) bool {
	for dominator := block; dominator != nil; dominator = dominator.Idom() {
		if len(dominator.Instrs) == 0 || len(dominator.Succs) != 2 {
			continue
		}
		branch, ok := dominator.Instrs[len(dominator.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		selected, arm, ok := selectArm(branch.Cond)
		if !ok || arm >= len(selected.States) {
			continue
		}
		state := selected.States[arm]
		done, ok := state.Chan.(*ssa.Call)
		taken := dominator.Succs[0]
		if ok && state.Dir == types.RecvOnly && ssaflow.CallMatchesSymbol(done.Common(), contextDone) &&
			ssaflow.CallReceiver(done.Common()) == ctx && len(taken.Preds) == 1 && taken.Dominates(block) {
			return true
		}
	}
	return false
}

// selectArm matches the test a select's dispatch makes: its chosen index
// equal to one arm.
func selectArm(condition ssa.Value) (*ssa.Select, int, bool) {
	comparison, ok := condition.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL {
		return nil, 0, false
	}
	index, ok := comparison.X.(*ssa.Extract)
	arm, constant := comparison.Y.(*ssa.Const)
	if !ok || !constant || index.Index != 0 || arm.Value == nil {
		return nil, 0, false
	}
	selected, ok := index.Tuple.(*ssa.Select)
	return selected, int(arm.Int64()), ok
}

// testedNonNil reports whether block runs only after a branch found value
// non-nil: the branch's non-nil successor has that branch as its only entry
// and dominates block.
func testedNonNil(value ssa.Value, block *ssa.BasicBlock) bool {
	for dominator := block; dominator != nil; dominator = dominator.Idom() {
		if len(dominator.Instrs) == 0 {
			continue
		}
		branch, ok := dominator.Instrs[len(dominator.Instrs)-1].(*ssa.If)
		if !ok || len(dominator.Succs) != 2 {
			continue
		}
		comparison, ok := branch.Cond.(*ssa.BinOp)
		if !ok || !comparesWithNil(comparison, value) {
			continue
		}
		nonNil := dominator.Succs[0]
		if comparison.Op == token.EQL {
			nonNil = dominator.Succs[1]
		}
		if len(nonNil.Preds) == 1 && nonNil.Dominates(block) {
			return true
		}
	}
	return false
}

func comparesWithNil(comparison *ssa.BinOp, value ssa.Value) bool {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false
	}
	return comparison.X == value && isNil(comparison.Y) || comparison.Y == value && isNil(comparison.X)
}

func isNil(value ssa.Value) bool {
	literal, ok := value.(*ssa.Const)
	return ok && literal.IsNil()
}

// concurrentWithRange requires every call of the closer to run at most once
// per call, and one call to run in a goroutine concurrently with the range on
// the same object.
func concurrentWithRange(functions []*ssa.Function, closer *ssa.Function, receive *ssa.UnOp) rangeReason {
	var calls []*ssa.Call
	for _, function := range functions {
		for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
			if call.Common().StaticCallee() != closer {
				continue
			}
			if ssaflow.BlockInCycle(call.Block()) {
				return rangeReasonCloserRepeated
			}
			calls = append(calls, call)
		}
	}
	ranged, rangeRoot, rangeLaunch := capturedObject(receiveBase(receive))
	if ranged == nil {
		return rangeReasonNotConcurrent
	}
	for _, call := range calls {
		object, root, launch := capturedObject(call.Common().Args[0])
		if object == nil || object != ranged || launch == nil || root != rangeRoot {
			continue
		}
		// The range runs in the root itself after the launch, or in another
		// goroutine the root launches.
		if rangeLaunch == nil && ssaflow.InstructionDominates(launch, receive) || rangeLaunch != nil && rangeLaunch != launch {
			return rangeReasonNone
		}
	}
	return rangeReasonNotConcurrent
}

func receiveBase(receive *ssa.UnOp) ssa.Value {
	load := receive.X.(*ssa.UnOp)
	return load.X.(*ssa.FieldAddr).X
}

// capturedObject resolves value to the object it names in the function that
// owns the captured variable, with that function and the instruction that
// launched value's goroutine, which is nil when value is in the owner itself.
// The variable must be written once, so every goroutine sees the same object.
func capturedObject(value ssa.Value) (ssa.Value, *ssa.Function, ssa.Instruction) {
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil, nil, nil
	}
	switch cell := load.X.(type) {
	case *ssa.Alloc:
		object, ok := ssaflow.WrittenOnceCell(cell)
		if !ok {
			return nil, nil, nil
		}
		return object, cell.Parent(), nil
	case *ssa.FreeVar:
		return capturedFromLaunch(cell)
	}
	return nil, nil, nil
}

// capturedFromLaunch maps a closure's captured variable to the enclosing
// function's cell, when the enclosing function launches the closure as a
// goroutine with a go statement or sync.WaitGroup.Go.
func capturedFromLaunch(capture *ssa.FreeVar) (ssa.Value, *ssa.Function, ssa.Instruction) {
	closure := capture.Parent()
	index := -1
	for position, candidate := range closure.FreeVars {
		if candidate == capture {
			index = position
		}
	}
	owner := closure.Parent()
	if index < 0 || owner == nil {
		return nil, nil, nil
	}
	for _, made := range ssaflow.InstructionsOf[*ssa.MakeClosure](owner) {
		if made.Fn != closure || index >= len(made.Bindings) {
			continue
		}
		cell, ok := made.Bindings[index].(*ssa.Alloc)
		if !ok {
			return nil, nil, nil
		}
		object, written := ssaflow.WrittenOnceCell(cell)
		launch := launchOf(made)
		if !written || launch == nil {
			return nil, nil, nil
		}
		return object, owner, launch
	}
	return nil, nil, nil
}

func launchOf(made *ssa.MakeClosure) ssa.Instruction {
	for _, use := range *made.Referrers() {
		switch use := use.(type) {
		case *ssa.Go:
			if use.Call.Value == made {
				return use
			}
		case *ssa.Call:
			arguments := use.Call.Args
			if ssaflow.CallMatchesSymbol(use.Common(), waitGroupGo) && len(arguments) == 2 && arguments[1] == made {
				return use
			}
		}
	}
	return nil
}
