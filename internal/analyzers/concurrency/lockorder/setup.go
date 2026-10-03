package lockorder

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Setup owns one function inventory and the metadata derived from it. Cutoff
// exposes no partial acquisition/ownership map. Identity, heap and type query
// internals remain separate from the charged census and summary boundary.
type lockFunctionSetup struct {
	instructions    []ssa.Instruction
	calls           []*ssa.Call
	defers          []*ssa.Defer
	returns         []*ssa.Return
	branches        []*ssa.If
	direct          map[ssa.Instruction]mutexEffect
	summaries       map[ssa.Instruction][]mutexEffect
	callerOwned     map[string]bool
	possibleWriters []*ssa.Defer
	hasAcquisition  bool
}

type lockSetupProof struct {
	ssaflow.Proof
	setup *lockFunctionSetup
}

func buildLockSetup(pass *analysis.Pass, function *ssa.Function, budget *ssaflow.SearchBudget) lockSetupProof {
	setup := &lockFunctionSetup{direct: map[ssa.Instruction]mutexEffect{}, summaries: map[ssa.Instruction][]mutexEffect{}}
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		setup.instructions = append(setup.instructions, instruction)
		if call, ok := instruction.(*ssa.Call); ok {
			setup.calls = append(setup.calls, call)
		}
		if deferred, ok := instruction.(*ssa.Defer); ok {
			setup.defers = append(setup.defers, deferred)
		}
		if returned, ok := instruction.(*ssa.Return); ok {
			setup.returns = append(setup.returns, returned)
		}
		if branch, ok := instruction.(*ssa.If); ok {
			setup.branches = append(setup.branches, branch)
		}
		if effect, known := directMutexEffectWithin(instruction, budget); known {
			setup.direct[instruction] = effect
		}
	}
	if budget.Exhausted() {
		return unavailableLockSetup()
	}
	if !setup.summarize(pass, budget) {
		return unavailableLockSetup()
	}
	setup.collectMetadata(budget)
	if budget.Exhausted() {
		return unavailableLockSetup()
	}
	if setup.hasAcquisition {
		setup.possibleWriters = setup.deferredWriterWitnesses(budget)
	}
	if budget.Exhausted() {
		return unavailableLockSetup()
	}
	return lockSetupProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk}, setup: setup}
}

func unavailableLockSetup() lockSetupProof {
	return lockSetupProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}}
}

func (setup *lockFunctionSetup) summarize(pass *analysis.Pass, pool *ssaflow.SearchBudget) bool {
	engine, _ := summaryKnowledge.Provider(pass).Concurrency()
	if engine == nil {
		return true
	}
	budget := pool.Within(ssaflow.SummaryBudget)
	for _, call := range setup.calls {
		if !pool.Spend() {
			return false
		}
		if _, direct := setup.direct[call]; direct {
			continue
		}
		// Builtins retain the flow's mutation checks; an empty summary would
		// bypass those checks on a read-locked owner.
		if _, builtin := call.Common().Value.(*ssa.Builtin); builtin {
			continue
		}
		summary := engine.AtCall(call, budget)
		if budget.Exhausted() {
			return false
		}
		if !summary.Complete() {
			continue
		}
		effects, complete := bindMutexEffects(call, summary.Operations, budget)
		if budget.Exhausted() {
			return false
		}
		if complete {
			setup.summaries[call] = effects
		}
	}
	return true
}

func (setup *lockFunctionSetup) collectMetadata(budget *ssaflow.SearchBudget) {
	type firstAction struct {
		operation mutexOperation
		position  token.Pos
	}
	first := map[string]firstAction{}
	for _, instruction := range setup.instructions {
		if !budget.Spend() {
			return
		}
		effects := setup.summaries[instruction]
		if effect, ok := setup.direct[instruction]; ok {
			effects = []mutexEffect{effect}
		}
		for _, effect := range effects {
			if !budget.Spend() {
				return
			}
			// No held state arises without a direct or fully bound acquisition.
			// Incomplete calls contribute ordering only when another lock is held.
			setup.hasAcquisition = setup.hasAcquisition || effect.operation == mutexAcquire
			current, exists := first[effect.identity]
			if instruction.Pos() != token.NoPos && (!exists || instruction.Pos() < current.position) {
				first[effect.identity] = firstAction{operation: effect.operation, position: instruction.Pos()}
			}
		}
	}
	setup.callerOwned = map[string]bool{}
	for identity, action := range first {
		if !budget.Spend() {
			return
		}
		setup.callerOwned[identity] = action.operation == mutexRelease
	}
}
