package lockorder

import (
	"fmt"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A read lock is shared: sync.RWMutex documents RLock as held by any number of
// readers at once. Writing to the object that lock protects while holding only
// RLock therefore races with every other reader, and unlike an order inversion
// it needs nothing unusual from a second goroutine -- two ordinary readers are
// enough.
//
// The claim is deliberately about the OBJECT, not about which field the lock
// guards. Proving that a mutex guards a particular field needs guard inference
// this analyzer does not do, and one struct may hold several independent guard
// domains: a mutex over one map, an atomic counter, and fields fixed at
// construction. What is provable without that inference is narrower: the
// receiver whose read lock is held is the receiver being mutated.
//
// A field also written in a call-free receiver method has an opaque caller
// synchronization contract and stays unknown; see readLockFieldEvidence.
// Other exclusions follow from the same rule and keep the claim honest. A value
// LOADED out of the owner is a different cell, so mutating it is not a write to
// the owner -- the distinction ssaflow.IdentitySource states for identity
// resolution. And an atomic update is a call rather than a store, so it never
// reaches here at all, which is correct: such a field is protected by atomics
// rather than by the lock.

// reportReadLockWrites reports a write to an object whose read lock is the only
// one held at this instruction.
func reportReadLockWrites(
	flow lockFlowContext, instruction ssa.Instruction, held, readHeld []string,
	lockValues map[string][]ssa.Value, possibleWriters []*ssa.Defer,
) {
	proof := proveReadLockWrite(instruction, held, readHeld, lockValues, possibleWriters, flow.setup.calls, flow.fieldEvidence)
	traceLockDiagnostic(flow.pass, check.LockReadLockWrite, instruction.Pos(), proof.lockDiagnosticProof)
	if proof.state != ssaflow.EvidenceProven {
		return
	}
	// Branch evidence can visit one write repeatedly. Keep proof and tracing
	// per state, but publish each instruction/lock witness only once. Unknown
	// states never reserve a witness, and a different held lock stays distinct.
	// https://github.com/apache/skywalking-rover/blob/e83d5925500a7e63dd55c080a9b1542d6cedaefb/pkg/tools/buffer/buffer.go#L644-L649
	witness := readLockWriteWitness{instruction: instruction, identity: proof.identity}
	if flow.readLockWrites[witness] {
		return
	}
	flow.readLockWrites[witness] = true
	source := syntax.SourceRange(flow.pass, instruction.Pos())
	check.Report(flow.pass, check.LockReadLockWrite, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(),
		Message: fmt.Sprintf("write while only the read lock %s is held", flow.lockName(proof.identity)),
		Related: flow.acquisitionEvidence(proof.identity),
	})
}

type readLockWriteWitness struct {
	instruction ssa.Instruction
	identity    string
}

type readLockWriteProof struct {
	lockDiagnosticProof
	identity string
}

// proveReadLockWrite selects the first reportable owner under current lock
// state. An exclusive or possible imported writer leaves this mutation unknown;
// neither is a field-protection contract. Unknown candidates retain the existing
// full owner-query order, while a reportable candidate ends the search.
func proveReadLockWrite(
	instruction ssa.Instruction, held, readHeld []string,
	lockValues map[string][]ssa.Value, possibleWriters []*ssa.Defer, calls []*ssa.Call, fields readLockFieldEvidence,
) readLockWriteProof {
	proof := readLockWriteProof{lockDiagnosticProof: lockDiagnosticProof{ssaflow.EvidenceDisproven, lockReasonNone}}
	for _, identity := range readHeld {
		// A lock the flow already transferred or released is no longer held,
		// even if it was taken for reading earlier on this path.
		if !slices.Contains(held, identity) {
			continue
		}
		for _, value := range lockValues[identity] {
			owner, ok := lockOwner(value)
			if !ok || !writeTargetsOwner(instruction, owner) {
				continue
			}
			// A field-only method exposes caller-supplied synchronization or separate
			// ownership that same-owner matching cannot resolve. This is uncertainty,
			// not proof that the setter is safe or that it uses a different mutex.
			// https://github.com/apache/skywalking-rover/blob/e83d5925500a7e63dd55c080a9b1542d6cedaefb/pkg/tools/buffer/buffer.go#L629-L649
			if guard := fields.guard(instruction); guard.state == ssaflow.EvidenceUnknown {
				proof = readLockWriteProof{guard, identity}
				continue
			}
			// A fresh wrapper can carry a borrowed map or slice. Exclusivity must
			// belong to the actual destination, not merely the lock's receiver.
			if storage, known := heapmodel.ExclusiveAt(readLockWriteDestination(instruction), instruction); known && storage.Local {
				proof = readLockWriteProof{lockDiagnosticProof{ssaflow.EvidenceDisproven, lockReasonPrivateWriteStorage}, identity}
				continue
			}
			// A writer guard may be owned by another object. Without guard
			// inference, any held exclusive lock makes the claim that only
			// readers serialize this write unknown, not proven safe.
			// https://github.com/rfjakob/gocryptfs/blob/842af4463989ee6808d397433e9aba8517e49c89/internal/fusefrontend/file.go#L409-L430
			if writeLockHeld(held, readHeld) {
				proof = readLockWriteProof{lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonExclusiveWriterUnknown}, identity}
				continue
			}
			if slices.ContainsFunc(possibleWriters, func(deferred *ssa.Defer) bool { return possibleWriterAt(deferred, instruction, calls) }) {
				proof = readLockWriteProof{lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonImportedWriterGuardUnknown}, identity}
				continue
			}
			return readLockWriteProof{lockDiagnosticProof{ssaflow.EvidenceProven, lockReasonReadLockWrite}, identity}
		}
	}
	return proof
}

// The caller supplies the completed setup census. Rechecking temporal and alias
// evidence here must not rediscover the same function body for every write.
func possibleWriterAt(deferred *ssa.Defer, instruction ssa.Instruction, calls []*ssa.Call) bool {
	if !ssaflow.InstructionDominates(deferred, instruction) {
		return false
	}
	_, _, writer, _ := mutexAction(deferred)
	for _, call := range calls {
		operation, _, receiver, direct := mutexAction(call)
		if direct && operation == mutexRelease && !readModeRelease(call) && heapmodel.MayAlias(receiver, writer) &&
			ssaflow.InstructionMayFollow(deferred, call) && ssaflow.InstructionMayFollow(call, instruction) {
			// An explicit intervening release defeats the possible-held guard;
			// the still-registered defer must not hide an unprotected write.
			return false
		}
	}
	return true
}

// An imported wrapper can acquire its embedded mutex while doing bookkeeping
// that the complete-effect summary cannot model. A deferred standard exclusive
// Unlock of that same wrapper is positive evidence of a possibly held writer.
// This is uncertainty, not guard-to-field inference or an acquisition effect;
// it must never enter order or recursive-lock proofs. Distinct wrapper receivers,
// known empty calls, and a release already executed provide no such evidence.
// https://github.com/rfjakob/gocryptfs/blob/842af4463989ee6808d397433e9aba8517e49c89/internal/fusefrontend/file.go#L418-L430
func (setup *lockFunctionSetup) deferredWriterWitnesses(budget *ssaflow.SearchBudget) []*ssa.Defer {
	var writers []*ssa.Defer
	for _, deferred := range setup.defers {
		if !budget.Spend() {
			return nil
		}
		effect, direct := setup.direct[deferred]
		operation, receiver := effect.operation, effect.receiver
		if !direct || operation != mutexRelease || readModeRelease(deferred) {
			continue
		}
		field, embedded := receiver.(*ssa.FieldAddr)
		if !embedded {
			continue
		}
		for _, call := range setup.calls {
			if !budget.Spend() {
				return nil
			}
			callee := call.Common().StaticCallee()
			if callee == nil || len(callee.Blocks) != 0 {
				continue
			}
			dominates := ssaflow.InstructionDominatesWithin(call, deferred, budget)
			if budget.Exhausted() {
				return nil
			}
			if !dominates {
				continue
			}
			if _, complete := setup.summaries[call]; complete {
				continue
			}
			// Only the embedded lock's exact wrapper may explain this deferred
			// writer release. Aliasing supplies an unknown witness, never a held
			// lock or ordering edge; interrupted setup discards all witnesses.
			calledReceiver := ssaflow.CallReceiver(call.Common())
			if calledReceiver != nil && heapmodel.MayAlias(calledReceiver, field.X) {
				writers = append(writers, deferred)
				break
			}
		}
	}
	return writers
}

// writeLockHeld reports whether this path also holds an exclusive lock.
func writeLockHeld(held, readHeld []string) bool {
	return slices.ContainsFunc(held, func(identity string) bool {
		return !slices.Contains(readHeld, identity)
	})
}

// lockOwner returns the value a lock is a field of. A package variable has no
// owner object, so what it protects is not decided here.
func lockOwner(value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // SSA values have several concrete forms.
	field, ok := value.(*ssa.FieldAddr)
	if !ok {
		return nil, false
	}
	return field.X, true
}

func writeTargetsOwner(instruction ssa.Instruction, owner ssa.Value) bool {
	destination := readLockWriteDestination(instruction)
	if destination == nil {
		return false
	}
	if _, store := instruction.(*ssa.Store); store {
		return addressWithinOwner(destination, owner)
	}
	// Containers loaded out of the owner's field retain their storage path.
	source, ok := ssaflow.IdentitySource(destination)
	return ok && addressWithinOwner(source, owner)
}

// readLockWriteDestination names the memory a store, map update or supported
// builtin mutates. Owner matching and exclusivity consume this same selection.
// copy writes only its first argument; its source remains a permitted read.
func readLockWriteDestination(instruction ssa.Instruction) ssa.Value { //nolint:ireturn // Preserve the exact SSA storage identity.
	switch operation := instruction.(type) {
	case *ssa.Store:
		return operation.Addr
	case *ssa.MapUpdate:
		return operation.Map
	case *ssa.Call:
		common := operation.Common()
		builtin, ok := common.Value.(*ssa.Builtin)
		if !ok || len(common.Args) == 0 {
			return nil
		}
		switch builtin.Name() {
		case "delete", "clear", "copy":
			return common.Args[0]
		}
	}
	return nil
}

// addressWithinOwner reports whether address selects a field or a constant
// index of owner. It never peels a load, so a value read out of the owner and
// then mutated is not counted as a write to the owner.
func addressWithinOwner(address, owner ssa.Value) bool {
	for address != nil {
		if heapmodel.MayAlias(address, owner) {
			return true
		}
		switch typed := address.(type) {
		case *ssa.FieldAddr:
			address = typed.X
		case *ssa.IndexAddr:
			// A read lock is shared, so two readers race only where they write
			// the SAME cell. Distinct elements are distinct memory, so an
			// element write at a caller-supplied index races only if two
			// readers can pass the same index -- which this analysis cannot
			// establish, and which a per-consumer cursor deliberately never
			// does. pyroscope's Tee hands each consumer its own index and
			// advances that cursor under the read lock:
			// https://github.com/grafana/pyroscope/blob/d1212251265e7dab4b03ef0d80af565f6d519e1b/pkg/iter/tee.go#L68-L76
			//
			// A constant index names one cell every reader shares, so it stays
			// reportable, as does a map update, which races whatever the key,
			// and a store to the field itself.
			if _, constant := typed.Index.(*ssa.Const); !constant {
				return false
			}
			// A slice header loaded out of the owner shares the owner's backing
			// array, so writing through it writes the owner. A pointer loaded
			// out of the owner is a different object, which is why only the
			// slice case peels its load.
			address = typed.X
			if _, slice := address.Type().Underlying().(*types.Slice); slice {
				if source, ok := ssaflow.IdentitySource(address); ok {
					address = source
				}
			}
		default:
			return false
		}
	}
	return false
}

// Receiver-field writes in a call-free method have no in-frame acquisition.
// Whether callers provide a guard or own the field is opaque. Declaration
// identity keeps this uncertainty on that field, not on its siblings or names.
// Interrupted collection supplies no negative field-guard evidence.
type readLockFieldEvidence struct {
	unscoped    map[*types.Var]bool
	unavailable bool
}

func collectReadLockFieldEvidence(functions []*ssa.Function, budget *ssaflow.SearchBudget) readLockFieldEvidence {
	result := readLockFieldEvidence{unscoped: map[*types.Var]bool{}}
	for _, function := range functions {
		if !budget.Spend() {
			return readLockFieldEvidence{unavailable: true}
		}
		if function.Signature.Recv() == nil || len(function.Params) == 0 || len(function.Blocks) == 0 {
			continue
		}
		// Stage witnesses until the entire method is known call-free. A later
		// call can provide synchronization that a preceding store does not show.
		var fields []*types.Var
		closed := true
		for instruction := range ssaflow.InstructionsWithin(function, budget) {
			if _, call := instruction.(ssa.CallInstruction); call {
				closed = false
				break
			}
			store, ok := instruction.(*ssa.Store)
			if !ok {
				continue
			}
			// A field of a loaded peer is a different object. Only the exact
			// receiver and its embedded addresses supply this declaration witness.
			path, known := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(budget),
				store.Addr, func(root ssa.Value) bool { return root == function.Params[0] })
			if known && path.Depth > 0 {
				if field := storedField(store); field != nil {
					fields = append(fields, field)
				}
			}
		}
		if budget.Exhausted() || budget.PoolExhausted() {
			return readLockFieldEvidence{unavailable: true}
		}
		if closed {
			for _, field := range fields {
				if !budget.Spend() {
					return readLockFieldEvidence{unavailable: true}
				}
				result.unscoped[field] = true
			}
		}
	}
	return result
}

func (fields readLockFieldEvidence) guard(instruction ssa.Instruction) lockDiagnosticProof {
	if fields.unavailable {
		return lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonLockStateBudgetExhausted}
	}
	if store, ok := instruction.(*ssa.Store); ok && fields.unscoped[storedField(store)] {
		return lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonFieldGuardUnknown}
	}
	return lockDiagnosticProof{ssaflow.EvidenceDisproven, lockReasonNone}
}

func storedField(store *ssa.Store) *types.Var {
	address, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	structure := syntax.PointerStruct(address.X.Type())
	if structure == nil || address.Field < 0 || address.Field >= structure.NumFields() {
		return nil
	}
	return structure.Field(address.Field)
}
