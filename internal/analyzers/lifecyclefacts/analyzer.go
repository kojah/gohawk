// Package lifecyclefacts exports cross-package function ownership summaries.
package lifecyclefacts

import (
	"go/types"
	"reflect"

	"github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	gosssa "golang.org/x/tools/go/ssa"
)

// Analyzer is an internal prerequisite shared by lifecycle analyzers.
var Analyzer = &analysis.Analyzer{
	Name:       "gohawklifecyclefacts",
	Doc:        "exports internal lifecycle ownership summaries",
	Requires:   []*analysis.Analyzer{buildssa.Analyzer},
	FactTypes:  []analysis.Fact{new(ssautil.LifecycleFact)},
	ResultType: reflect.TypeOf(ssautil.LifecycleSummaries{}),
	Run:        run,
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	summaries := make(ssautil.LifecycleSummaries, len(functions))
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
		if fact != (ssautil.LifecycleFact{}) {
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
				if fact, ok := ssautil.ImportLifecycleFact(pass, instruction); ok {
					summaries[common.StaticCallee()] = fact
				}
			}
		}
	}
	return summaries, nil
}

// FactFor returns a memoized local or previously imported dependency summary.
func FactFor(pass *analysis.Pass, instruction gosssa.Instruction) (ssautil.LifecycleFact, bool) {
	common := ssautil.InstructionCall(instruction)
	if pass == nil || common == nil || common.StaticCallee() == nil {
		return ssautil.LifecycleFact{}, false
	}
	if summaries, ok := pass.ResultOf[Analyzer].(ssautil.LifecycleSummaries); ok {
		if fact, found := summaries[common.StaticCallee()]; found {
			return fact, true
		}
	}
	return ssautil.LifecycleFact{}, false
}

// OwnsArgument reports whether a summarized ownership mask covers target.
func OwnsArgument(pass *analysis.Pass, instruction gosssa.Instruction, target gosssa.Value, selectMask func(ssautil.LifecycleFact) uint64) bool {
	fact, ok := FactFor(pass, instruction)
	return ok && ssautil.FactOwnsArgument(instruction, target, selectMask(fact))
}

// StoresInEscapingReceiver reports a summarized field transfer only when the
// caller-visible receiver itself outlives the call.
func StoresInEscapingReceiver(pass *analysis.Pass, instruction gosssa.Instruction, target gosssa.Value) bool {
	common := ssautil.InstructionCall(instruction)
	receiver := ssautil.CallReceiver(common)
	if receiver == nil || !ssautil.ExternallyOwnedValue(receiver) && !ssautil.ValueEscapes(receiver) {
		return false
	}
	return OwnsArgument(pass, instruction, target, func(fact ssautil.LifecycleFact) uint64 { return fact.ReceiverStore })
}

func summarize(pass *analysis.Pass, function *gosssa.Function) ssautil.LifecycleFact {
	var fact ssautil.LifecycleFact
	for index, parameter := range function.Params {
		if !ownershipCapableType(parameter.Type()) {
			continue
		}
		bit := uint64(1) << index
		if ownsOnEveryReturn(function, parameter, func(instruction gosssa.Instruction) bool {
			common := ssautil.InstructionCall(instruction)
			if common != nil && ssautil.SameValue(common.Value, parameter) {
				return true
			}
			imported, ok := ssautil.ImportLifecycleFact(pass, instruction)
			return ok && ssautil.FactOwnsArgument(instruction, parameter, imported.Invoked)
		}) {
			fact.Invoked |= bit
		}
		for method, target := range map[string]*uint64{
			"Close": &fact.Closed, "Finalize": &fact.Finalized, "Release": &fact.Released,
			"Shutdown": &fact.Shutdown, "Stop": &fact.Stopped, "Wait": &fact.Waited,
		} {
			if ownsOnEveryReturn(function, parameter, func(instruction gosssa.Instruction) bool {
				common := ssautil.InstructionCall(instruction)
				if common != nil && ssautil.CallName(common) == method && ssautil.ValueDerivesFrom(ssautil.CallReceiver(common), parameter, map[gosssa.Value]bool{}) {
					return true
				}
				imported, ok := ssautil.ImportLifecycleFact(pass, instruction)
				return ok && ssautil.FactOwnsArgument(instruction, parameter, imported.MethodMask(method))
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
