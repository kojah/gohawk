package lockorder

import (
	"go/token"
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/ssaflow"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"

	proofs "github.com/kojah/gohawk/internal/proof"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// An acquisition a callee makes is deliberately not discharged from the
// call: the callee's acquisitions are summarized per lock class, so one
// record can stand for several parameters, and a fresh argument in one
// position must not excuse an acquisition fed by a shared argument in
// another.
//
// An acquisition on an object no other goroutine can reach orders nothing:
// the lock cannot be contended, so taking it while another lock is held
// says nothing about the order the program takes the two in elsewhere. The
// common shape is initialization, a fresh object locked while a registry
// lock is held and published only afterwards, whose reverse of the steady
// state order is not a deadlock. The proof is a precondition in two halves.
// In the function that locks, the points-to graph shows the object has not
// escaped at the acquisition; it is either a local allocation that is
// published later, or a parameter. For a parameter, every caller in the
// package must hand in a fresh local that has not escaped at the call, and
// the function must be unexported, so that no caller is unseen. A local
// that never escapes at all is deliberately not covered: the existing
// fixtures report an inversion between two purely local locks, and this
// change does not revisit that policy.
// https://github.com/ozontech/file.d, gauge, and gopcua/opcua are the
// dogfood shapes: a job, gauge, or channel registered under a global lock
// after its own lock is taken.

// exclusiveCallers consumes the same complete private caller census as return
// contracts, so an unseen callback or asynchronous use cannot prove exclusivity.
type exclusiveCallers struct {
	pass  *analysis.Pass
	sites map[*ssa.Function]conditionalCallerSet
	// Only completed caller proofs are cached; interrupted requests can retry.
	exclusive map[exclusiveKey]exclusiveProof
}

type exclusiveProof struct {
	state     proofs.EvidenceState
	reason    lockReason
	parameter int
}

type exclusiveKey struct {
	function *ssa.Function
	index    int
}

func newExclusiveCallers(pass *analysis.Pass, sites map[*ssa.Function]conditionalCallerSet) *exclusiveCallers {
	return &exclusiveCallers{pass: pass, sites: sites, exclusive: map[exclusiveKey]exclusiveProof{}}
}

// parameterExclusive reports whether every call of the function in the
// package passes, at the parameter's position, a fresh local object that has
// not escaped at the call. Exported functions, methods, functions with escaped
// uses, and incomplete or empty caller sets remain unknown. A fresh direct
// caller cannot establish what a callback or asynchronous caller hands in.
func (callers *exclusiveCallers) parameterExclusive(function *ssa.Function, index int, budget *proofs.SearchBudget) exclusiveProof {
	unknown := exclusiveProof{reason: lockReasonExclusiveOwnershipUnknown}
	cutoff := exclusiveProof{reason: lockReasonLockStateBudgetExhausted}
	// Admission also applies to cached answers: an interrupted flow cannot use
	// a prior request's positive witness to continue recording order evidence.
	if !budget.Spend() {
		return cutoff
	}
	key := exclusiveKey{function: function, index: index}
	if answer, ok := callers.exclusive[key]; ok {
		return answer
	}
	object := function.Object()
	entry := callers.sites[function]
	if object == nil || object.Exported() || entry.Escaped || len(entry.Calls) == 0 {
		callers.exclusive[key] = unknown
		return unknown
	}
	for _, call := range entry.Calls {
		if !budget.Spend() {
			return cutoff
		}
		if index < 0 || index >= len(call.Common().Args) {
			callers.exclusive[key] = unknown
			return unknown
		}
		// Graph construction and observation are owned by heapmodel. This request
		// charges selection of each completed caller, not graph-internal work.
		exclusive, ok := heapmodel.ExclusiveAt(call.Common().Args[index], call)
		if !ok || !exclusive.Local {
			callers.exclusive[key] = unknown
			return unknown
		}
	}
	proof := exclusiveProof{state: proofs.EvidenceProven, reason: lockReasonExclusiveParameterFromFreshCallers, parameter: index}
	callers.exclusive[key] = proof
	return proof
}

// acquisitionExclusive decides whether an acquisition orders nothing
// because its object is exclusively owned at the instruction, and traces
// the half of the proof that decided it.
func (callers *exclusiveCallers) acquisitionExclusive(
	function *ssa.Function, instruction ssa.Instruction, receiver ssa.Value, budget *proofs.SearchBudget,
) exclusiveProof {
	proof := callers.proveAcquisitionExclusive(function, instruction, receiver, budget)
	if proof.state != proofs.EvidenceProven {
		return proof
	}
	probe := analysisTrace.For(callers.pass, "lockorder", string(check.LockContradictoryOrder), instruction.Pos())
	if probe.Enabled() {
		step := analysisTrace.Step{Reason: proof.reason.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: instruction.Pos()}
		if proof.reason == lockReasonExclusiveParameterFromFreshCallers {
			// The proof selected this parameter; metadata is read only when tracing.
			step.Details = map[string]string{"parameter": strconv.Itoa(proof.parameter), "callers": strconv.Itoa(len(callers.sites[function].Calls))}
		}
		probe.Decision(step)
	}
	return proof
}

func (callers *exclusiveCallers) proveAcquisitionExclusive(
	function *ssa.Function, instruction ssa.Instruction, receiver ssa.Value, budget *proofs.SearchBudget,
) exclusiveProof {
	unknown := exclusiveProof{reason: lockReasonExclusiveOwnershipUnknown}
	if !budget.Spend() {
		return exclusiveProof{reason: lockReasonLockStateBudgetExhausted}
	}
	if receiver == nil {
		return unknown
	}
	exclusive, ok := heapmodel.ExclusiveAt(receiver, instruction)
	if !ok {
		return unknown
	}
	switch {
	case exclusive.Local && exclusive.Published:
		return exclusiveProof{state: proofs.EvidenceProven, reason: lockReasonExclusiveObjectBeforePublication}
	case exclusive.Local:
		return unknown
	default:
		return callers.parameterExclusive(function, exclusive.Parameter, budget)
	}
}

// An initial publication is not unescaped ownership. It leaves the blocking
// role of the first acquisition unknown under a uniquely matching owner writer.
// Only that ordering edge is declined; the acquired lock stays held afterwards.
type publicationGuardProof struct {
	lockDiagnosticProof
	identity string
}

func (flow lockFlowContext) initialPublicationGuard(instruction ssa.Instruction, receiver ssa.Value, state lockFlowState) publicationGuardProof {
	proof := publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{proofs.EvidenceDisproven, lockReasonNone}}
	effect, direct := flow.setup.direct[instruction]
	allocation, fresh := receiver.(*ssa.Alloc)
	if !direct || effect.operation != mutexAcquire || effect.acquired.read || !fresh || allocation.Block() != instruction.Block() {
		return proof
	}
	owner := initialPublicationOwner(allocation, instruction, flow.budget)
	if flow.budget.Exhausted() || flow.budget.PoolExhausted() {
		return publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{proofs.EvidenceUnknown, lockReasonLockStateBudgetExhausted}}
	}
	if owner == nil {
		return proof
	}
	// More than one writer on this owner supplies no unique publication guard.
	// A reader or a possibly released writer cannot supply this boundary either.
	matched := ""
	for _, identity := range state.held {
		if !flow.budget.Spend() {
			return publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{proofs.EvidenceUnknown, lockReasonLockStateBudgetExhausted}}
		}
		if slices.Contains(state.readHeld, identity) || flow.unprovenRelease[identity] {
			continue
		}
		if flow.writerMatchesOwner(identity, owner) {
			if matched != "" && matched != identity {
				return proof
			}
			matched = identity
		}
	}
	if flow.budget.Exhausted() || flow.budget.PoolExhausted() {
		return publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{proofs.EvidenceUnknown, lockReasonLockStateBudgetExhausted}}
	}
	if matched != "" {
		return publicationGuardProof{lockDiagnosticProof{proofs.EvidenceUnknown, lockReasonInitialPublicationUnknown}, matched}
	}
	return proof
}

// writerMatchesOwner checks exact receiver identity only; it does not establish
// that readers honor this writer. The caller checks cutoff before using it.
func (flow lockFlowContext) writerMatchesOwner(identity string, owner ssa.Value) bool {
	for _, value := range flow.lockValues[identity] {
		if !flow.budget.Spend() {
			return false
		}
		guardOwner, known := lockOwner(value)
		if known && ssaflow.StructurallyIdenticalWithin(guardOwner, owner, flow.budget) {
			return true
		}
	}
	return false
}

// Address selection and loads may separate a fresh allocation from its map
// publication and first Lock. Calls, stores, sends, branches or a second map
// publication break that closed interval: another participant could see or lock
// the value before the acquisition. Unsupported shapes retain ordering evidence.
func initialPublicationOwner(allocation *ssa.Alloc, acquisition ssa.Instruction, budget *proofs.SearchBudget) ssa.Value {
	first := ssaflow.InstructionIndexWithin(allocation, budget)
	last := ssaflow.InstructionIndexWithin(acquisition, budget)
	if first < 0 || last <= first {
		return nil
	}
	var owner ssa.Value
	for _, instruction := range acquisition.Block().Instrs[first+1 : last] {
		if !budget.Spend() {
			return nil
		}
		switch operation := instruction.(type) {
		case *ssa.FieldAddr, *ssa.DebugRef:
		case *ssa.UnOp:
			if operation.Op != token.MUL {
				return nil
			}
		case *ssa.MapUpdate:
			if owner != nil || operation.Value != allocation || operation.Key == allocation {
				return nil
			}
			source, known := ssaflow.IdentitySource(operation.Map)
			field, selected := source.(*ssa.FieldAddr)
			if !known || !selected {
				return nil
			}
			owner = field.X
		default:
			return nil
		}
	}
	return owner
}
