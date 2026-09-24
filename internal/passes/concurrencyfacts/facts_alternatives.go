package concurrencyfacts

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

// A function whose branches have different synchronization effects has no
// single linear fact. Its bounded path alternatives are published instead,
// each with its own effects, the conditions that select it, and what it
// returns where that is certain. Conditions travel by formal parameter
// position, like effects. A condition inside the function is published as a
// numbered opaque condition: an importer can relate two tests of it within
// one alternative but learns nothing about its value. Constants travel only
// in kinds that round-trip exactly; any other comparison becomes opaque.

// FactAlternative is one published path alternative.
type FactAlternative struct {
	Effects            []Effect
	Workers            []WorkerEffect
	CancellationInputs []int
	Conditions         []FactCondition
	Returned           []FactReturned
}

// FactCondition is one condition that selects an alternative. Parameter is
// the tested formal, or -1 for a condition inside the function, numbered by
// Internal. Constant, when present, is what the tested value is compared with.
type FactCondition struct {
	Parameter int
	Internal  int
	Constant  FactConstant
	Holds     bool
	Implied   bool
}

// FactReturned says result Index is Constant, or cannot be nil when Constant
// is absent.
type FactReturned struct {
	Index    int
	Constant FactConstant
}

// FactConstantKind names how a constant was published.
type FactConstantKind uint8

const (
	FactConstantAbsent FactConstantKind = iota
	FactConstantNil
	FactConstantBool
	FactConstantInt
	FactConstantString
)

// FactConstant is a constant in a form that round-trips exactly.
type FactConstant struct {
	Kind  FactConstantKind
	Exact string
}

// exportAlternatives publishes every path alternative, or none.
func exportAlternatives(function *ssa.Function, paths []Summary) (Fact, bool) {
	fact := Fact{Version: factVersion}
	internal := map[string]int{}
	for _, path := range paths {
		body, ok := exportSummary(function, path)
		if !ok {
			return Fact{Version: factVersion}, false
		}
		alternative := FactAlternative{Effects: body.Effects, Workers: body.Workers, CancellationInputs: body.CancellationInputs}
		for _, condition := range path.Conditions {
			alternative.Conditions = append(alternative.Conditions, exportCondition(function, condition, internal))
		}
		for _, returned := range path.Returned {
			published, ok := exportConstant(returned.Constant)
			if returned.Constant != nil && !ok {
				continue
			}
			alternative.Returned = append(alternative.Returned, FactReturned{Index: returned.Index, Constant: published})
		}
		fact.Alternatives = append(fact.Alternatives, alternative)
	}
	return fact, true
}

func exportCondition(function *ssa.Function, condition Condition, internal map[string]int) FactCondition {
	published, exact := exportConstant(condition.Compared)
	if len(condition.Context) == 0 && (condition.Compared == nil || exact) {
		for index, parameter := range function.Params {
			if parameter == condition.Value {
				return FactCondition{Parameter: index, Constant: published, Holds: condition.Holds, Implied: condition.Implied}
			}
		}
	}
	// Two tests of one inner condition must share a number, so the key keeps
	// what makes them the same test: the value, its context, and the constant.
	key := fmt.Sprintf("%p#%v#%v", condition.Value, condition.Context, condition.Compared)
	number, seen := internal[key]
	if !seen {
		number = len(internal)
		internal[key] = number
	}
	return FactCondition{Parameter: -1, Internal: number, Holds: condition.Holds, Implied: condition.Implied}
}

func exportConstant(value *ssa.Const) (FactConstant, bool) {
	switch {
	case value == nil:
		return FactConstant{}, true
	case value.IsNil():
		return FactConstant{Kind: FactConstantNil}, true
	case value.Value == nil:
		return FactConstant{}, false
	}
	switch value.Value.Kind() {
	case constant.Bool:
		return FactConstant{Kind: FactConstantBool, Exact: value.Value.ExactString()}, true
	case constant.Int:
		return FactConstant{Kind: FactConstantInt, Exact: value.Value.ExactString()}, true
	case constant.String:
		return FactConstant{Kind: FactConstantString, Exact: value.Value.ExactString()}, true
	default:
		return FactConstant{}, false
	}
}

// importConstant rebuilds a published constant with the importer's type.
func importConstant(published FactConstant, of types.Type) (*ssa.Const, bool) {
	switch published.Kind {
	case FactConstantNil:
		return ssa.NewConst(nil, of), true
	case FactConstantBool:
		return ssa.NewConst(constant.MakeBool(published.Exact == "true"), of), true
	case FactConstantInt:
		value := constant.MakeFromLiteral(published.Exact, token.INT, 0)
		return ssa.NewConst(value, of), value.Kind() == constant.Int
	case FactConstantString:
		text, err := strconv.Unquote(published.Exact)
		return ssa.NewConst(constant.MakeString(text), of), err == nil
	default:
		return nil, false
	}
}

// bindAlternatives binds every published alternative at call, or none.
func (engine *Engine) bindAlternatives(call ssa.CallInstruction, fact Fact) Summary {
	unknown := Summary{Reason: ReasonBodyUnavailable}
	if len(fact.Alternatives) > maxProtocolPaths {
		return unknown
	}
	arguments := engine.resolvedCommon(call).Args
	var paths []Summary
	for _, alternative := range fact.Alternatives {
		path := engine.bindDeclaration(call, Fact{
			Version: fact.Version, Effects: alternative.Effects, Workers: alternative.Workers,
			CancellationInputs: alternative.CancellationInputs,
		})
		if !composableLinear(path) {
			return path
		}
		for _, published := range alternative.Conditions {
			condition, ok := importCondition(call, arguments, published)
			if !ok {
				return unknown
			}
			path.Conditions = append(path.Conditions, condition)
		}
		path.Returned = importReturned(call, alternative.Returned)
		paths = append(paths, path)
	}
	return finishPaths(paths)
}

// importCondition binds a published condition. A parameter condition tests
// the caller's argument. An inner condition is opaque: its context is this
// call site followed by a negative marker naming the condition, so two tests
// of it relate within this call and never across calls or conditions.
func importCondition(call ssa.CallInstruction, arguments []ssa.Value, published FactCondition) (Condition, bool) {
	if published.Parameter < 0 {
		marker := token.Pos(-1 - published.Internal)
		return Condition{
			Value: call.Common().Value, Holds: published.Holds, Implied: published.Implied,
			Context: []token.Pos{call.Pos(), marker},
		}, published.Internal >= 0
	}
	if published.Parameter >= len(arguments) {
		return Condition{}, false
	}
	argument := arguments[published.Parameter]
	condition := Condition{Value: argument, Holds: published.Holds, Implied: published.Implied}
	if published.Constant.Kind != FactConstantAbsent {
		compared, ok := importConstant(published.Constant, argument.Type())
		if !ok {
			return Condition{}, false
		}
		condition.Compared = compared
	}
	return condition, true
}

func importReturned(call ssa.CallInstruction, published []FactReturned) []Returned {
	results := call.Common().Signature().Results()
	var returned []Returned
	for _, fact := range published {
		if fact.Index < 0 || fact.Index >= results.Len() {
			continue
		}
		entry := Returned{Index: fact.Index}
		if fact.Constant.Kind != FactConstantAbsent {
			constant, ok := importConstant(fact.Constant, results.At(fact.Index).Type())
			if !ok {
				continue
			}
			entry.Constant = constant
		}
		returned = append(returned, entry)
	}
	return returned
}
