// Package lifecyclefacts exports cross-package function ownership summaries.
package lifecyclefacts

import (
	"go/types"
	"reflect"
	"strconv"

	"github.com/kojah/gohawk/internal/analysisutil/ssa"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	gosssa "golang.org/x/tools/go/ssa"
)

// Analyzer is an internal prerequisite shared by lifecycle analyzers.
var Analyzer = &analysis.Analyzer{
	Name:       "gohawklifecyclefacts",
	Doc:        "exports internal lifecycle ownership summaries",
	Requires:   []*analysis.Analyzer{buildssa.Analyzer},
	FactTypes:  []analysis.Fact{new(Fact)},
	ResultType: reflect.TypeOf(summarySet{}),
	Run:        run,
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	summaries := make(summarySet, len(functions))
	for _, function := range functions {
		object := function.Object()
		// Only exported functions can be called from a package that imports this
		// fact. Skipping private dependency helpers keeps the prerequisite linear
		// in the externally visible API instead of every transitive SSA body.
		if object == nil || !object.Exported() || len(function.Params) > 64 || len(function.Blocks) == 0 {
			continue
		}
		fact := summarize(pass, function)
		summaries[function] = fact
		if fact != (Fact{}) {
			pass.ExportObjectFact(object, &fact)
		}
	}
	// Facts belong to this prerequisite analyzer, so import dependency facts
	// here and expose them through the result consumed by sibling analyzers.
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				common := ssautil.InstructionCall(instruction)
				if common == nil || common.StaticCallee() == nil {
					continue
				}
				if fact, ok := importFact(pass, instruction); ok {
					summaries[common.StaticCallee()] = fact
				}
			}
		}
	}
	return summaries, nil
}

// factFor returns a memoized local or previously imported dependency summary.
func factFor(pass *analysis.Pass, instruction gosssa.Instruction) (Fact, bool) {
	common := ssautil.InstructionCall(instruction)
	if pass == nil || common == nil || common.StaticCallee() == nil {
		return Fact{}, false
	}
	if summaries, ok := pass.ResultOf[Analyzer].(summarySet); ok {
		if fact, found := summaries[common.StaticCallee()]; found {
			return fact, true
		}
	}
	return Fact{}, false
}

// OwnsArgument reports whether a summarized ownership mask covers target.
func OwnsArgument(pass *analysis.Pass, analyzer, check string, instruction gosssa.Instruction, target gosssa.Value, selectMask func(Fact) ParameterMask) bool {
	fact, ok := factFor(pass, instruction)
	mask := selectMask(fact)
	owned := ok && factOwnsArgument(instruction, target, mask)
	emitSummaryTrace(pass, analyzer, check, instruction, target, mask, owned)
	return owned
}

// StoresInEscapingReceiver reports a summarized field transfer only when the
// caller-visible receiver itself outlives the call.
func StoresInEscapingReceiver(pass *analysis.Pass, analyzer, check string, instruction gosssa.Instruction, target gosssa.Value) bool {
	common := ssautil.InstructionCall(instruction)
	fact, summarized := factFor(pass, instruction)
	if !summarized || !factOwnsArgument(instruction, target, fact.ReceiverStore) {
		return false
	}
	receiver := ssautil.CallReceiver(common)
	escapes := receiver != nil && (ssautil.ExternallyOwnedValue(receiver) || ssautil.ValueEscapes(receiver))
	reason := analysisTrace.ReasonReceiverStoreTransfer
	outcome := analysisTrace.OutcomeAccepted
	if !escapes {
		reason = analysisTrace.ReasonReceiverDoesNotEscape
		outcome = analysisTrace.OutcomeRejected
	}
	if analysisTrace.Enabled(analyzer, check) {
		analysisTrace.Emit(pass, analysisTrace.Event{Analyzer: analyzer, Check: check, Phase: "evidence", Reason: reason, Outcome: outcome, Pos: instruction.Pos(), Function: functionName(instruction), Details: summaryDetails(instruction, target, fact.ReceiverStore)})
	}
	if !escapes {
		return false
	}
	return true
}

func emitSummaryTrace(pass *analysis.Pass, analyzer, check string, instruction gosssa.Instruction, target gosssa.Value, mask ParameterMask, owned bool) {
	if !analysisTrace.Enabled(analyzer, check) {
		return
	}
	outcome := analysisTrace.OutcomeRejected
	if owned {
		outcome = analysisTrace.OutcomeAccepted
	}
	analysisTrace.Emit(pass, analysisTrace.Event{Analyzer: analyzer, Check: check, Phase: "evidence", Reason: analysisTrace.ReasonLifecycleSummary, Outcome: outcome, Pos: instruction.Pos(), Function: functionName(instruction), Details: summaryDetails(instruction, target, mask)})
}

func summaryDetails(instruction gosssa.Instruction, target gosssa.Value, mask ParameterMask) map[string]string {
	details := map[string]string{"mask": strconv.FormatUint(uint64(mask), 16)}
	if target != nil && target.Type() != nil {
		details["target_type"] = target.Type().String()
	}
	if common := ssautil.InstructionCall(instruction); common != nil && common.StaticCallee() != nil {
		details["callee"] = common.StaticCallee().String()
	}
	return details
}

func functionName(instruction gosssa.Instruction) string {
	if instruction == nil || instruction.Parent() == nil {
		return ""
	}
	return instruction.Parent().String()
}

func summarize(pass *analysis.Pass, function *gosssa.Function) Fact {
	var fact Fact
	for index, parameter := range function.Params {
		if !ownershipCapableType(parameter.Type()) {
			continue
		}
		bit := parameterMaskFor(index)
		if ownsOnEveryReturn(function, parameter, func(instruction gosssa.Instruction) bool {
			common := ssautil.InstructionCall(instruction)
			if common != nil && ssautil.SameValue(common.Value, parameter) {
				return true
			}
			imported, ok := importFact(pass, instruction)
			return ok && factOwnsArgument(instruction, parameter, imported.Invoked)
		}) {
			fact.Invoked |= bit
		}
		for method, target := range map[string]*ParameterMask{
			"Close": &fact.Closed, "Finalize": &fact.Finalized, "Release": &fact.Released,
			"Shutdown": &fact.Shutdown, "Stop": &fact.Stopped, "Wait": &fact.Waited,
		} {
			if ownsOnEveryReturn(function, parameter, func(instruction gosssa.Instruction) bool {
				common := ssautil.InstructionCall(instruction)
				if common != nil && ssautil.CallName(common) == method && ssautil.ValueDerivesFrom(ssautil.CallReceiver(common), parameter, map[gosssa.Value]bool{}) {
					return true
				}
				imported, ok := importFact(pass, instruction)
				return ok && factOwnsArgument(instruction, parameter, imported.MethodMask(method))
			}) {
				*target |= bit
			}
		}
		if returnedOwnerOnEveryReturn(function, parameter) {
			fact.ReturnedOwner |= bit
		}
		if index > 0 && storedInReceiverOnEveryReturn(function, function.Params[0], parameter) {
			fact.ReceiverStore |= bit
		}
	}
	return fact
}

func ownsOnEveryReturn(function *gosssa.Function, parameter gosssa.Value, owns func(gosssa.Instruction) bool) bool {
	return !ssautil.UnownedReturnFromEntryAssumingNonNil(function, parameter, owns)
}

func returnedOwnerOnEveryReturn(function *gosssa.Function, parameter gosssa.Value) bool {
	if !canReturnOwner(function.Signature.Results()) {
		return false
	}
	return !ssautil.UnownedReturnFromEntryAllow(function, func(gosssa.Instruction) bool { return false }, func(returned *gosssa.Return) bool {
		return ssautil.ReturnedValueOwnsValue(returned, parameter) || allResultsNil(returned)
	})
}

func canReturnOwner(results *types.Tuple) bool {
	for index := range results.Len() {
		if types.Identical(results.At(index).Type(), types.Universe.Lookup("error").Type()) {
			continue
		}
		if ownershipCapableType(results.At(index).Type()) {
			return true
		}
	}
	return false
}

func ownershipCapableType(value types.Type) bool {
	switch value.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Signature, *types.Map, *types.Slice, *types.Chan, *types.Struct, *types.Array:
		return true
	default:
		return false
	}
}

func allResultsNil(returned *gosssa.Return) bool {
	if len(returned.Results) == 0 {
		return false
	}
	for _, result := range returned.Results {
		if !ssautil.DefinitelyNil(result) && result.Type().String() != "error" {
			return false
		}
	}
	return true
}

func storedInReceiverOnEveryReturn(function *gosssa.Function, receiver, parameter gosssa.Value) bool {
	return !ssautil.UnownedReturnFromEntry(function, func(instruction gosssa.Instruction) bool {
		store, ok := instruction.(*gosssa.Store)
		if !ok || !ssautil.ValueDerivesFrom(store.Val, parameter, map[gosssa.Value]bool{}) {
			return false
		}
		field, ok := store.Addr.(*gosssa.FieldAddr)
		return ok && ssautil.ValueDerivesFrom(field.X, receiver, map[gosssa.Value]bool{})
	})
}
