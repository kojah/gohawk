package producerlifecycle

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// A branch on the owner's own state before a send to a service loop may be a
// lifecycle guard, such as robfig/cron's running flag. The guard protects the
// send only if the flag cannot change and the loop cannot stop between the
// check and the send. This file proves that from one owner mutex: the guard is
// read and the send made while it is held, every write of the guard field
// holds it exclusively, and every stop signal holds it exclusively too. An
// unlocked read or write, or a loop that also stops on a context, is positive
// evidence the guard does not protect the send. Anything this package cannot
// see, such as a method call in the condition or a field whose address
// escapes, leaves the guard unknown.
// Real-world contrast: gocronx-team/cron read and wrote its running flag with
// no lock, so Schedule could pass the check as Stop ended the loop:
// https://github.com/gocronx-team/cron/blob/75c065182ba5e11e7e0ff90ff0ee7e29f22254ea/cron.go#L191-L211
// robfig/cron holds runningMu for the check, the send, and the stop instead.

type guardVerdict uint8

const (
	guardAbsent guardVerdict = iota
	guardProtects
	guardUnsynchronized
	guardUnknown
)

var (
	mutexLock     = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Lock"})
	mutexUnlock   = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Unlock"})
	rwLock        = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Lock"})
	rwUnlock      = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Unlock"})
	rwReadLock    = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RLock"})
	rwReadUnlock  = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RUnlock"})
	exclusiveLock = []syntax.Symbol{mutexLock, rwLock}
	sharedLock    = []syntax.Symbol{mutexLock, rwLock, rwReadLock}
	anyUnlock     = []syntax.Symbol{mutexUnlock, rwUnlock, rwReadUnlock}
)

// proveLifecycleGuard classifies the owner-state branches that dominate send.
func proveLifecycleGuard(inventory *channelInventory, send *ssa.Send, owner ssa.Value, stops []ssa.Value) guardVerdict {
	fields, opaque := dominatingGuardFields(send, owner)
	switch {
	case opaque:
		return guardUnknown
	case len(fields) == 0:
		return guardAbsent
	}
	held := heldMutexes(send, owner, sharedLock)
	if len(held) == 0 {
		// The guard and the send race with any writer of the guard.
		return guardUnsynchronized
	}
	verdict := guardUnsynchronized
	for _, mutex := range held {
		switch mutexGuards(inventory, mutex, fields, stops) {
		case guardProtects:
			return guardProtects
		case guardUnknown:
			verdict = guardUnknown
		case guardAbsent, guardUnsynchronized:
			// Another held mutex may still protect the send.
		}
	}
	return verdict
}

// dominatingGuardFields returns the owner fields that branch conditions before
// send read directly. A call on the owner in a condition is opaque.
func dominatingGuardFields(send *ssa.Send, owner ssa.Value) ([]*types.Var, bool) {
	var fields []*types.Var
	for dominator := send.Block().Idom(); dominator != nil; dominator = dominator.Idom() {
		if len(dominator.Instrs) == 0 {
			continue
		}
		branch, ok := dominator.Instrs[len(dominator.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		read, opaque := ownerReads(branch.Cond, owner, 4)
		if opaque {
			return nil, true
		}
		fields = append(fields, read...)
	}
	return fields, false
}

// ownerReads walks a branch condition a few operations deep. The depth bound
// keeps this a structural test of one expression; phis are not followed.
func ownerReads(value, owner ssa.Value, depth int) ([]*types.Var, bool) {
	if depth == 0 || value == nil {
		return nil, false
	}
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL {
		if address, ok := load.X.(*ssa.FieldAddr); ok && address.X == owner {
			return []*types.Var{fieldOf(address)}, false
		}
	}
	switch value := value.(type) {
	case *ssa.Phi:
		return nil, false
	case *ssa.Call:
		for _, argument := range value.Call.Args {
			if readsOwner(argument, owner, 2) {
				return nil, true
			}
		}
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return nil, false
	}
	var fields []*types.Var
	for _, operand := range instruction.Operands(nil) {
		read, opaque := ownerReads(*operand, owner, depth-1)
		if opaque {
			return nil, true
		}
		fields = append(fields, read...)
	}
	return fields, false
}

// mutexGuards checks that every write of the guard fields and every stop
// signal holds mutex exclusively.
func mutexGuards(inventory *channelInventory, mutex *types.Var, fields []*types.Var, stops []ssa.Value) guardVerdict {
	verdict := guardProtects
	for _, field := range fields {
		verdict = weaker(verdict, writesHold(inventory, field, mutex))
	}
	for _, stop := range stops {
		verdict = weaker(verdict, stopHolds(inventory, stop, mutex))
	}
	return verdict
}

func weaker(current, next guardVerdict) guardVerdict {
	if current == guardUnsynchronized || next == guardUnsynchronized {
		return guardUnsynchronized
	}
	if current == guardUnknown || next == guardUnknown {
		return guardUnknown
	}
	return guardProtects
}

// writesHold requires every store to field to hold mutex exclusively on the
// same owner. Initializing a freshly allocated owner needs no lock, because no
// other goroutine can see it yet in the allocating function's own stores.
func writesHold(inventory *channelInventory, field, mutex *types.Var) guardVerdict {
	addresses, closed := inventory.addresses[field]
	if !closed {
		return guardUnknown
	}
	verdict := guardProtects
	for _, address := range addresses {
		for _, use := range *address.Referrers() {
			switch use := use.(type) {
			case *ssa.DebugRef:
			case *ssa.UnOp:
				if use.Op != token.MUL {
					return guardUnknown
				}
			case *ssa.Store:
				if use.Addr != address {
					return guardUnknown
				}
				if _, fresh := address.X.(*ssa.Alloc); fresh {
					continue
				}
				if !slices.Contains(heldMutexes(use, address.X, exclusiveLock), mutex) {
					verdict = guardUnsynchronized
				}
			default:
				return guardUnknown
			}
		}
	}
	return verdict
}

// stopHolds requires every send or close on a stop channel to hold mutex
// exclusively. A context can be cancelled with no lock at all, so a loop that
// stops on Done is never protected by the guard.
func stopHolds(inventory *channelInventory, stop ssa.Value, mutex *types.Var) guardVerdict {
	load, ok := stop.(*ssa.UnOp)
	if !ok {
		return guardUnsynchronized
	}
	address, ok := load.X.(*ssa.FieldAddr)
	if !ok {
		return guardUnknown
	}
	owned := inventory.fields[fieldOf(address)]
	if owned == nil || owned.signalsHidden() {
		return guardUnknown
	}
	for _, signal := range owned.signals {
		base := signalOwner(signal)
		if base == nil {
			return guardUnknown
		}
		if !slices.Contains(heldMutexes(signal, base, exclusiveLock), mutex) {
			return guardUnsynchronized
		}
	}
	return guardProtects
}

// heldMutexes returns the owner mutex fields locked before target on every
// path, with no unlock that can run between the lock and target. A deferred
// unlock runs at return, so it does not end the hold.
func heldMutexes(target ssa.Instruction, owner ssa.Value, locks []syntax.Symbol) []*types.Var {
	var held []*types.Var
	for _, lock := range ssaflow.InstructionsOf[*ssa.Call](target.Parent()) {
		field, ok := ownerMutexCall(lock.Common(), owner, locks)
		if !ok || !ssaflow.InstructionDominates(lock, target) || unlockedBetween(lock, target, owner, field) {
			continue
		}
		held = append(held, field)
	}
	return held
}

func unlockedBetween(lock *ssa.Call, target ssa.Instruction, owner ssa.Value, field *types.Var) bool {
	for _, instruction := range ssaflow.InstructionsReachableAfter(lock) {
		unlock, ok := instruction.(*ssa.Call)
		if !ok {
			continue
		}
		if released, ok := ownerMutexCall(unlock.Common(), owner, anyUnlock); ok && released == field &&
			slices.Contains(ssaflow.InstructionsReachableAfter(unlock), target) {
			return true
		}
	}
	return false
}

func ownerMutexCall(call *ssa.CallCommon, owner ssa.Value, symbols []syntax.Symbol) (*types.Var, bool) {
	if !ssaflow.CallMatchesAnySymbol(call, symbols...) {
		return nil, false
	}
	address, ok := ssaflow.CallReceiver(call).(*ssa.FieldAddr)
	if !ok || address.X != owner {
		return nil, false
	}
	return fieldOf(address), true
}

// signalOwner returns the owner value a stop signal's channel was loaded from.
func signalOwner(signal ssa.Instruction) ssa.Value {
	var channel ssa.Value
	switch signal := signal.(type) {
	case *ssa.Send:
		channel = signal.Chan
	case *ssa.Call:
		if len(signal.Call.Args) == 1 {
			channel = signal.Call.Args[0]
		}
	case *ssa.Select:
		// A select that signals has other arms; which one runs is not proven.
		return nil
	}
	load, ok := channel.(*ssa.UnOp)
	if !ok {
		return nil
	}
	address, ok := load.X.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	return address.X
}

func fieldOf(address *ssa.FieldAddr) *types.Var {
	structure, _ := addressedStruct(address.X.Type())
	if structure == nil {
		return nil
	}
	return structure.Field(address.Field)
}
