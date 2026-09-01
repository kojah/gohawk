// Package lifecyclefacts resolves lifecycle evidence across source and package
// boundaries. It combines memoized local SSA proofs with conservative exported
// summaries, and keeps missing summaries distinct from disproved ownership.
package lifecyclefacts

import (
	"go/types"
	"reflect"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	ssa "golang.org/x/tools/go/ssa"
)

// Analyzer is an internal prerequisite shared by lifecycle analyzers.
var Analyzer = &analysis.Analyzer{
	Name:       "gohawklifecyclefacts",
	Doc:        "exports internal lifecycle ownership summaries",
	Requires:   []*analysis.Analyzer{buildssa.Analyzer},
	FactTypes:  []analysis.Fact{new(Fact)},
	ResultType: reflect.TypeFor[summarySet](),
	Run:        run,
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
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
				common := ssaflow.InstructionCall(instruction)
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
func factFor(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	common := ssaflow.InstructionCall(instruction)
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

func summarize(pass *analysis.Pass, function *ssa.Function) Fact {
	var fact Fact
	// A fact is exported only when the action is unavoidable on every normal
	// return. Each mask is therefore proved independently; evidence for Close,
	// for example, must never make an unrelated Wait or return-transfer claim true.
	for index, parameter := range function.Params {
		if !ownershipCapableType(parameter.Type()) {
			continue
		}
		bit := parameterMaskFor(index)
		if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			if common != nil && ssaflow.SameValue(common.Value, parameter) {
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
			if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
				common := ssaflow.InstructionCall(instruction)
				if common != nil && ssaflow.CallName(common) == method &&
					ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), parameter, map[ssa.Value]bool{}) {
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

func ownsOnEveryReturn(function *ssa.Function, parameter ssa.Value, owns func(ssa.Instruction) bool) bool {
	return !ssaflow.UnownedReturnFromEntryAssumingNonNil(function, parameter, owns)
}

func returnedOwnerOnEveryReturn(function *ssa.Function, parameter ssa.Value) bool {
	if !canReturnOwner(function.Signature.Results()) {
		return false
	}
	return !ssaflow.UnownedReturnFromEntryAllow(function, func(ssa.Instruction) bool { return false }, func(returned *ssa.Return) bool {
		return ssaflow.ReturnedValueOwnsValue(returned, parameter) || allResultsNil(returned)
	})
}

func canReturnOwner(results *types.Tuple) bool {
	for result := range results.Variables() {
		if types.Identical(result.Type(), types.Universe.Lookup("error").Type()) {
			continue
		}
		if ownershipCapableType(result.Type()) {
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

func allResultsNil(returned *ssa.Return) bool {
	if len(returned.Results) == 0 {
		return false
	}
	for _, result := range returned.Results {
		if !ssaflow.DefinitelyNil(result) && result.Type().String() != "error" {
			return false
		}
	}
	return true
}

func storedInReceiverOnEveryReturn(function *ssa.Function, receiver, parameter ssa.Value) bool {
	return !ssaflow.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
		store, ok := instruction.(*ssa.Store)
		if !ok || !ssaflow.ValueDerivesFrom(store.Val, parameter, map[ssa.Value]bool{}) {
			return false
		}
		field, ok := store.Addr.(*ssa.FieldAddr)
		return ok && ssaflow.ValueDerivesFrom(field.X, receiver, map[ssa.Value]bool{})
	})
}
