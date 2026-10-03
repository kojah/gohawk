package lockorder

import (
	"go/token"
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/ssaflow"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"

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
	// exclusive caches the answer per function and parameter.
	exclusive map[exclusiveKey]bool
}

type exclusiveKey struct {
	function *ssa.Function
	index    int
}

func newExclusiveCallers(pass *analysis.Pass, sites map[*ssa.Function]conditionalCallerSet) *exclusiveCallers {
	return &exclusiveCallers{pass: pass, sites: sites, exclusive: map[exclusiveKey]bool{}}
}

// parameterExclusive reports whether every call of the function in the
// package passes, at the parameter's position, a fresh local object that has
// not escaped at the call. Exported functions, methods, functions with escaped
// uses, and incomplete or empty caller sets remain unknown. A fresh direct
// caller cannot establish what a callback or asynchronous caller hands in.
func (callers *exclusiveCallers) parameterExclusive(function *ssa.Function, index int) bool {
	key := exclusiveKey{function: function, index: index}
	if answer, ok := callers.exclusive[key]; ok {
		return answer
	}
	callers.exclusive[key] = false
	object := function.Object()
	entry := callers.sites[function]
	sites := entry.Calls
	if object == nil || object.Exported() || entry.Escaped || len(sites) == 0 {
		return false
	}
	for _, call := range sites {
		if index >= len(call.Common().Args) {
			return false
		}
		exclusive, ok := heapmodel.ExclusiveAt(call.Common().Args[index], call)
		if !ok || !exclusive.Local {
			return false
		}
	}
	callers.exclusive[key] = true
	return true
}

// acquisitionExclusive decides whether an acquisition orders nothing
// because its object is exclusively owned at the instruction, and traces
// the half of the proof that decided it.
func (callers *exclusiveCallers) acquisitionExclusive(function *ssa.Function, instruction ssa.Instruction, receiver ssa.Value) bool {
	if receiver == nil {
		return false
	}
	exclusive, ok := heapmodel.ExclusiveAt(receiver, instruction)
	if !ok {
		return false
	}
	probe := analysisTrace.For(callers.pass, "lockorder", string(check.LockContradictoryOrder), instruction.Pos())
	switch {
	case exclusive.Local && exclusive.Published:
		probe.Decision(analysisTrace.Step{
			Reason: lockReasonExclusiveObjectBeforePublication.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: instruction.Pos(),
		})
		return true
	case exclusive.Local:
		return false
	case callers.parameterExclusive(function, exclusive.Parameter):
		probe.Decision(analysisTrace.Step{
			Reason: lockReasonExclusiveParameterFromFreshCallers.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: instruction.Pos(),
			Details: map[string]string{"parameter": strconv.Itoa(exclusive.Parameter), "callers": strconv.Itoa(len(callers.sites[function].Calls))},
		})
		return true
	}
	return false
}

// An initial publication is not unescaped ownership. It leaves the blocking
// role of the first acquisition unknown under a uniquely matching owner writer.
// Only that ordering edge is declined; the acquired lock stays held afterwards.
type publicationGuardProof struct {
	lockDiagnosticProof
	identity string
}

func (flow lockFlowContext) initialPublicationGuard(instruction ssa.Instruction, receiver ssa.Value, state lockFlowState) publicationGuardProof {
	proof := publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{ssaflow.EvidenceDisproven, lockReasonNone}}
	effect, direct := flow.setup.direct[instruction]
	allocation, fresh := receiver.(*ssa.Alloc)
	if !direct || effect.operation != mutexAcquire || effect.acquired.read || !fresh || allocation.Block() != instruction.Block() {
		return proof
	}
	owner := initialPublicationOwner(allocation, instruction, flow.budget)
	if flow.budget.Exhausted() || flow.budget.PoolExhausted() {
		return publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonLockStateBudgetExhausted}}
	}
	if owner == nil {
		return proof
	}
	// More than one writer on this owner supplies no unique publication guard.
	// A reader or a possibly released writer cannot supply this boundary either.
	matched := ""
	for _, identity := range state.held {
		if !flow.budget.Spend() {
			return publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonLockStateBudgetExhausted}}
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
		return publicationGuardProof{lockDiagnosticProof: lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonLockStateBudgetExhausted}}
	}
	if matched != "" {
		return publicationGuardProof{lockDiagnosticProof{ssaflow.EvidenceUnknown, lockReasonInitialPublicationUnknown}, matched}
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
func initialPublicationOwner(allocation *ssa.Alloc, acquisition ssa.Instruction, budget *ssaflow.SearchBudget) ssa.Value {
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
