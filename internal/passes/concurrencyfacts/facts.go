package concurrencyfacts

// Facts carry complete ordered effects across the vet package boundary.
// Absence, incompatible versions, local allocations, captures, and unknown
// effects are not empty summaries. No serialized token.Pos or SSA value crosses
// a package boundary; imported evidence is attributed to the importing call.

import (
	"go/types"
	"reflect"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/ssaflow"

	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

const (
	factVersion  = 3
	exportBudget = 2000
)

// Effect is an ordered operation on a formal parameter (receiver at index zero).
type Effect struct {
	Kind      Kind
	Parameter int
	Fields    []int
}

// Fact records a complete sequence, including a proven empty sequence.
type Fact struct {
	Version int
	Effects []Effect
}

// AFact marks the versioned concurrency summary for go/analysis serialization.
func (*Fact) AFact() {}

// GobEncode encodes the fact through factcodec.
func (fact *Fact) GobEncode() ([]byte, error) { return factcodec.Encode(fact) }

// GobDecode decodes the fact through factcodec.
func (fact *Fact) GobDecode(data []byte) error { return factcodec.Decode(data, fact) }

// Analyzer exports complete effects and provides a shared engine to consumers.
var Analyzer = &analysis.Analyzer{
	Name: "gohawkconcurrencyfacts", Doc: "exports bounded ordered synchronization effects",
	Requires: []*analysis.Analyzer{buildssa.Analyzer}, FactTypes: []analysis.Fact{new(Fact)},
	ResultType: reflect.TypeFor[*Engine](), Run: run,
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	engine := NewEngine()
	engine.facts = make(map[*types.Func]Fact)
	for _, imported := range pass.AllObjectFacts() {
		object, ok := imported.Object.(*types.Func)
		fact, valid := imported.Fact.(*Fact)
		if ok && valid && fact.Version == factVersion {
			engine.facts[object] = *fact
		}
	}
	for _, function := range functions {
		object := function.Object()
		if object == nil || !object.Exported() {
			continue
		}
		probe := trace.For(pass, "concurrencyfacts", "", function.Pos())
		probe.Candidate(trace.Step{Reason: "summarizing-concurrency", Outcome: trace.OutcomeObserved})
		result := engine.Function(function, ssaflow.NewSearchBudget(exportBudget))
		fact, ok := exportSummary(function, result)
		outcome, reason := trace.OutcomeUnknown, "concurrency-export-unknown"
		if ok {
			pass.ExportObjectFact(object, &fact)
			outcome, reason = trace.OutcomeAccepted, "concurrency-export-complete"
		}
		probe.Decision(trace.Step{Reason: reason, Outcome: outcome, Pos: function.Pos()})
	}
	return engine, nil
}

func exportSummary(function *ssa.Function, result Summary) (Fact, bool) {
	fact := Fact{Version: factVersion}
	if !result.Complete() || result.Spawn != nil || len(result.deferred) != 0 {
		return fact, false
	}
	for _, operation := range result.Operations {
		resource := operation.Resource
		path, projected := embeddedPath(resource.Value)
		index := -1
		for i, parameter := range function.Params {
			if !resource.Indirect && (parameter == resource.Value ||
				projected && parameter == path.Root && MutexPointer(resource.Value.Type())) {
				index = i
				break
			}
		}
		if index < 0 {
			return fact, false
		}
		effect := Effect{Kind: operation.Kind, Parameter: index}
		if projected && path.Depth > 0 {
			effect.Fields = append([]int(nil), path.Fields[:path.Depth]...)
		}
		fact.Effects = append(fact.Effects, effect)
	}
	return fact, true
}

func (engine *Engine) importedCall(call ssa.CallInstruction, function *ssa.Function) Summary {
	unknown := Summary{Reason: "protocol-body-unavailable"}
	object, ok := function.Object().(*types.Func)
	if !ok {
		return unknown
	}
	fact, ok := engine.facts[object]
	if !ok {
		return unknown
	}
	return engine.bindDeclaration(call, fact)
}

func (engine *Engine) bindDeclaration(call ssa.CallInstruction, fact Fact) Summary {
	unknown := Summary{Reason: "protocol-body-unavailable"}
	if fact.Version != factVersion || len(fact.Effects) > maxOperations {
		return unknown
	}
	var result Summary
	for _, effect := range fact.Effects {
		if !engine.budget.Spend() {
			return Summary{Reason: "protocol-budget-exhausted"}
		}
		if effect.Parameter < 0 || effect.Parameter >= len(call.Common().Args) || effect.Kind > CondWait {
			return unknown
		}
		value := call.Common().Args[effect.Parameter]
		if len(effect.Fields) > 0 {
			var found bool
			value, found = engine.importedField(call, value, effect.Fields)
			if !found || effect.Kind != Lock && effect.Kind != Unlock {
				return Summary{Reason: "protocol-field-binding-unknown"}
			}
		}
		if reason := engine.appendOperation(&result, effect.Kind, value, call.Pos()); reason != "" {
			return Summary{Reason: reason}
		}
	}
	return result
}
