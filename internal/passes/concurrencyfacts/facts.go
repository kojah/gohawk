package concurrencyfacts

// Facts carry ordered effects and explicit binding requirements across the vet package boundary.
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
	factVersion  = 6
	exportBudget = 2000
)

// Effect is an ordered operation on a formal parameter (receiver at index zero).
type Effect struct {
	Kind      Kind
	Parameter int
	Fields    []int
}

// WorkerEffect is one exact child launch with effects on the declaration's
// formal parameters. Prefix counts synchronous effects before the launch;
// each call instantiates a separate child with its own call-site identity.
type WorkerEffect struct {
	Effects []Effect
	Prefix  int
}

// Fact records an exhaustive sequence, including an empty sequence. Any
// CancellationInputs must be discharged before that sequence is usable proof.
type Fact struct {
	Version int
	Effects []Effect
	Workers []WorkerEffect
	// CancellationInputs are formal indices whose context/cancel contracts
	// require binding to exact standard-library origins before consumption.
	CancellationInputs []int
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
		probe.Candidate(trace.Step{Reason: ReasonSummarizing.String(), Outcome: trace.OutcomeObserved})
		result := engine.linear.Function(function, ssaflow.NewSearchBudget(exportBudget))
		fact, ok := exportSummary(function, result)
		outcome, reason := trace.OutcomeUnknown, ReasonExportUnknown
		if ok {
			pass.ExportObjectFact(object, &fact)
			outcome, reason = trace.OutcomeAccepted, ReasonExportComplete
		}
		probe.Decision(trace.Step{Reason: reason.String(), Outcome: outcome, Pos: function.Pos()})
	}
	return engine, nil
}

// A conditional cancellation summary is publishable, but not yet usable as
// proof. Publish its input requirements with its effects, including for an
// empty sequence; otherwise an opaque Done implementation could become pure
// merely by crossing a package boundary. Local origins cannot be exported.
func exportSummary(function *ssa.Function, result Summary) (Fact, bool) {
	fact := Fact{Version: factVersion}
	if !composableLinear(result) || len(result.Paths) != 0 || len(result.Workers) > maxWorkers || len(result.deferred) != 0 ||
		result.operationCount() > maxOperations {
		return fact, false
	}
	for _, input := range result.CancellationInputs {
		effect, ok := exportEffect(function, Operation{Resource: input})
		if !ok || len(effect.Fields) != 0 || !input.Cancellation {
			return Fact{Version: factVersion}, false
		}
		fact.CancellationInputs = append(fact.CancellationInputs, effect.Parameter)
	}
	for _, operation := range result.Operations {
		effect, ok := exportEffect(function, operation)
		if !ok {
			return fact, false
		}
		fact.Effects = append(fact.Effects, effect)
	}
	for _, worker := range result.Workers {
		if len(worker.Alternatives) != 0 || worker.Prefix < 0 || worker.Prefix > len(fact.Effects) ||
			worker.Spawn == nil && !worker.Site.IsValid() {
			return Fact{Version: factVersion}, false
		}
		entry := WorkerEffect{Prefix: worker.Prefix}
		for _, operation := range worker.Operations {
			effect, ok := exportEffect(function, operation)
			if !ok {
				return Fact{Version: factVersion}, false
			}
			entry.Effects = append(entry.Effects, effect)
		}
		fact.Workers = append(fact.Workers, entry)
	}
	return fact, true
}

func exportEffect(function *ssa.Function, operation Operation) (Effect, bool) {
	resource := operation.Resource
	path, projected := embeddedPath(resource.Value)
	if resource.Projection.Depth > 0 {
		path, projected = resource.Projection, true
	}
	for index, parameter := range function.Params {
		if resource.Indirect || parameter != resource.Value &&
			(!projected || parameter != path.Root || !MutexPointer(resource.Value.Type())) {
			continue
		}
		effect := Effect{Kind: operation.Kind, Parameter: index}
		if projected && path.Depth > 0 {
			effect.Fields = append([]int(nil), path.Fields[:path.Depth]...)
		}
		return effect, true
	}
	return Effect{}, false
}

func (engine *Engine) importedCall(call ssa.CallInstruction, function *ssa.Function) Summary {
	unknown := Summary{Reason: ReasonBodyUnavailable}
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
	unknown := Summary{Reason: ReasonBodyUnavailable}
	arguments := engine.resolvedCommon(call).Args
	if fact.Version != factVersion || len(fact.Effects)+len(fact.CancellationInputs) > maxOperations || len(fact.Workers) > maxWorkers {
		return unknown
	}
	var result Summary
	// Declaration parameter positions use direct-call convention. A resolved
	// interface invocation must include its unboxed receiver at position zero
	// before either cancellation requirements or ordered effects are rebound.
	for _, index := range fact.CancellationInputs {
		if !engine.budget.Spend() || index < 0 || index >= len(arguments) {
			return unknown
		}
		resource, ok := engine.reference(arguments[index])
		if !ok || !resource.Cancellation {
			return Summary{Reason: ReasonContextBindingUnknown}
		}
		requireCancellation(&result, []Reference{resource})
	}
	for _, effect := range fact.Effects {
		if reason := engine.bindEffect(&result, call, effect); reason != ReasonNone {
			return Summary{Reason: reason}
		}
	}
	for _, entry := range fact.Workers {
		if entry.Prefix < 0 || entry.Prefix > len(result.Operations) {
			return unknown
		}
		worker := WorkerSummary{Prefix: entry.Prefix, Site: call.Pos()}
		for _, effect := range entry.Effects {
			var effects Summary
			if reason := engine.bindEffect(&effects, call, effect); reason != ReasonNone {
				return Summary{Reason: reason}
			}
			worker.Operations = append(worker.Operations, effects.Operations[0])
		}
		result.Workers = append(result.Workers, worker)
	}
	if result.operationCount() > maxOperations {
		return Summary{Reason: ReasonSummaryLimit}
	}
	return finishCancellation(result)
}

func (engine *Engine) bindEffect(result *Summary, call ssa.CallInstruction, effect Effect) Reason {
	if !engine.budget.Spend() {
		return ReasonBudgetExhausted
	}
	arguments := engine.resolvedCommon(call).Args
	if effect.Parameter < 0 || effect.Parameter >= len(arguments) || effect.Kind > ReadUnlock {
		return ReasonBodyUnavailable
	}
	value := arguments[effect.Parameter]
	if len(effect.Fields) > 0 {
		var found bool
		value, found = engine.importedField(call, value, effect.Fields)
		if !found || effect.Kind != Lock && effect.Kind != Unlock && effect.Kind != ReadLock && effect.Kind != ReadUnlock {
			return ReasonFieldBindingUnknown
		}
	}
	return engine.appendOperation(result, effect.Kind, value, call.Pos())
}
