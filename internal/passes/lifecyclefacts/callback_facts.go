package lifecyclefacts

import (
	"sync"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Callbacks a callee in this package invokes are known as soon as the callee
// is summarized, and callees are summarized before their callers, but facts
// are exported only once the whole package is summarized. A helper that
// forwards its callback to a sibling that calls it would otherwise see no
// fact for the sibling and claim nothing. ubuntu/decorate's LogFuncOnError
// hands its callback to LogFuncOnErrorContext, which calls it, so
// `defer decorate.LogFuncOnError(file.Close)` closes the file:
// https://github.com/ubuntu/decorate/blob/b76bb81d1209ba92b56c759d828f75c19479a444/decorate.go#L35-L45
//
// Only the invocation claims read these summaries. Every other claim keeps
// importing exported facts, because some are completed after the package is
// summarized. An unexported sibling is never summarized, so its invocation
// claims are proved from its body when first asked; a callee already being
// proved, which only a call cycle reaches again, claims nothing.

var localFacts = struct {
	sync.Mutex
	byPass map[*analysis.Pass]map[*ssa.Function]Fact
}{byPass: map[*analysis.Pass]map[*ssa.Function]Fact{}}

func rememberLocalFact(pass *analysis.Pass, function *ssa.Function, fact Fact) {
	localFacts.Lock()
	defer localFacts.Unlock()
	facts := localFacts.byPass[pass]
	if facts == nil {
		facts = map[*ssa.Function]Fact{}
		localFacts.byPass[pass] = facts
	}
	fact.signature = function.Signature
	facts[function] = fact
}

func forgetLocalFacts(pass *analysis.Pass) {
	localFacts.Lock()
	defer localFacts.Unlock()
	delete(localFacts.byPass, pass)
}

// invocationDischarges proves whether the function invokes its parameter on
// every normal return, and whether it does so synchronously: directly, or by
// handing it to a callee whose summary invokes that argument.
func invocationDischarges(pass *analysis.Pass, function *ssa.Function, index int, parameter ssa.Value) []Discharge {
	var discharges []Discharge
	invokes := func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		if common != nil && heapmodel.NewStorage(nil).Same(common.Value, parameter).Proven() {
			return true
		}
		imported, ok := callbackFact(pass, instruction)
		return ok && factOwnsExactArgument(instruction, parameter, imported.InvokedParameters())
	}
	if ownsOnEveryReturn(function, parameter, invokes) {
		discharges = append(discharges, Discharge{Parameter: index, Method: InvokeMethod})
	}
	if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
		return synchronouslyInvokesParameter(pass, instruction, parameter)
	}) {
		discharges = append(discharges, Discharge{Parameter: index, Method: SynchronousInvokeMethod})
	}
	return discharges
}

// callbackFact returns the callee's summary for questions about which
// callbacks it invokes: its exported fact, the summary this pass already
// computed for a callee in the same package, or, for a same-package callee
// that is not summarized, its invocation claims proved from its body.
func callbackFact(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	if fact, ok := importFact(pass, instruction); ok {
		return fact, true
	}
	callee := ssaflow.ResolvedCallee(ssaflow.InstructionCall(instruction))
	if callee == nil || pass == nil || callee.Pkg == nil || callee.Pkg.Pkg != pass.Pkg || len(callee.Blocks) == 0 {
		return Fact{}, false
	}
	localFacts.Lock()
	facts := localFacts.byPass[pass]
	if facts == nil {
		facts = map[*ssa.Function]Fact{}
		localFacts.byPass[pass] = facts
	}
	if fact, ok := facts[callee]; ok {
		localFacts.Unlock()
		return fact, true
	}
	// A placeholder with no claims answers a call cycle back into this callee.
	facts[callee] = Fact{signature: callee.Signature}
	localFacts.Unlock()
	fact := Fact{signature: callee.Signature}
	for index, parameter := range callee.Params {
		if index < 64 && ownershipCapableType(parameter.Type()) {
			fact.Discharges = append(fact.Discharges, invocationDischarges(pass, callee, index, parameter)...)
		}
	}
	localFacts.Lock()
	facts[callee] = fact
	localFacts.Unlock()
	return fact, true
}
