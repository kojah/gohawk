// Package concurrencyfacts composes complete, bounded synchronization effects.
// It provides ordered evidence, not deadlock policy or schedule exploration.
package concurrencyfacts

import (
	"go/token"
	"go/types"
	"slices"
	"sync"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Summaries preserve a bounded ordered sequence over symbolic parameters and
// captures. Exported facts use parameter positions, never process-local SSA
// pointers. Divergent branches and overflowing sequences remain unknown.
const maxOperations = 32

// Kind identifies a synchronization event with an exact resource.
type Kind uint8

const (
	Send Kind = iota
	Receive
	Close
	GroupAdd
	GroupDone
	GroupWait
	Lock
	Unlock
	CondWait
	// Cancel requests cancellation; it neither joins a worker nor proves that
	// Done has closed before the call returns.
	Cancel
	ReadLock
	ReadUnlock
)

// Reference names an exact resource or a symbolic captured cell.
type Reference struct {
	Value    ssa.Value
	Indirect bool
	// Projection is a parameter-relative embedded address carried through a
	// helper that never directly selects that field. Public bound queries
	// materialize it to an existing caller address before returning evidence.
	// With Indirect, it denotes a channel slot whose stable contents must be
	// proved by heapmodel before exposing a bound resource identity.
	Projection ssaflow.EmbeddedFieldPath
	// Cancellation names a context's Done signal, not an ordinary channel.
	// Value is a constructor call once bound, or a symbolic context/cancel input.
	Cancellation bool
}

// Operation retains execution order and source/call-site provenance.
type Operation struct {
	Kind     Kind
	Resource Reference
	Source   token.Pos
	Site     token.Pos
}

// SelectArm is one possible communication performed by a select. A default
// arm performs no communication; it must never be treated as a blocking event.
type SelectArm struct {
	Operation Operation
	Default   bool
	// StateIndex is the original SSA select index. An exact nil-channel arm
	// can be omitted without changing the indices used by the dispatch.
	StateIndex int
	// Sequence is the complete ordered effect sequence for this arm, from
	// function entry through its normal return. Nil when not proven.
	Sequence []Operation
	Complete bool
}

// SelectChoice records mutually exclusive arms at their position in the
// enclosing sequence. It is evidence about the alternatives, not permission
// to use the prefix as a complete protocol proof.
type SelectChoice struct {
	Arms   []SelectArm
	Prefix int
	Site   token.Pos
	Worker *ssa.Go
}

// Summary is the ordered synchronization effect of one function or call.
// Consumers decide on Completeness, never on the shape of Operations alone:
// an empty operation list is evidence only when the summary is complete.
// Reason explains an incomplete summary and is stable trace vocabulary.
// Returned slices are immutable. Workers are symbolic child templates until a
// root binds them to call sites; they are never synchronous effects.
type Summary struct {
	cutoff *summaryCutoff
	// Paths contains every bounded acyclic alternative. Each entry is a
	// complete linear summary or an exhaustive worker choice; never a prefix.
	// Linear consumers must decline the enclosing nonzero Reason.
	Paths      []Summary
	Operations []Operation
	deferred   []Operation
	Workers    []WorkerSummary
	Choices    []SelectChoice
	// CancellationInputs are requirements on context and cancel-function
	// inputs. Until bound to known constructors, their calls may have opaque
	// effects. They survive composition even when Done's result is unused.
	CancellationInputs []Reference
	// AlternativesComplete is true only after every select continuation and
	// the enclosing function body have been accounted for. Reason remains
	// nonzero so linear consumers cannot mistake alternatives for one path.
	AlternativesComplete bool
	Reason               Reason
}

// WorkerSummary keeps one child's complete ordered effects and its launch
// point relative to the parent's synchronization events.
type WorkerSummary struct {
	Operations   []Operation
	Alternatives [][]Operation
	Spawn        *ssa.Go
	Site         token.Pos
	Prefix       int
	// Branches distinguishes exhaustive ordinary branch paths from select
	// arms, whose correspondence is additionally checked against Choices.
	Branches bool
}

// Completeness is the contract a consumer acts on. Only a complete summary
// rules anything out; an incomplete one may hide any effect at all.
type Completeness uint8

const (
	// Incomplete means some instruction, callee, or binding could not be
	// summarized, so missing effects cannot be ruled out. Reason names why.
	Incomplete Completeness = iota
	// CompleteNoEffects means every instruction was accounted for and none of
	// them synchronizes: the function is proved effect-free.
	CompleteNoEffects
	// CompleteWithEffects means every instruction was accounted for and the
	// root has an operation or a child launch. Each sequence retains its order.
	CompleteWithEffects
)

// Completeness classifies the summary for its consumers. It is derived from
// the same fields the builder writes, so it cannot disagree with Reason.
func (summary Summary) Completeness() Completeness {
	switch {
	case summary.Reason != ReasonNone || len(summary.Paths) != 0 || !summary.CancellationBound():
		return Incomplete
	case len(summary.Operations) == 0 && len(summary.Workers) == 0:
		return CompleteNoEffects
	default:
		return CompleteWithEffects
	}
}

// Complete reports whether every effect was accounted for, with or without
// effects. Callers that need the distinction switch on Completeness.
func (summary Summary) Complete() bool {
	return summary.Completeness() != Incomplete
}

// Engine caches evidence for one package. Concurrent consumers are serialized
// at the public query boundary; each query must supply its own work budget.
type Engine struct {
	cutoff    *summaryCutoff
	mu        sync.Mutex
	summaries *ssaflow.FunctionSummaries[Summary]
	linear    *ssaflow.FunctionSummaries[Summary]
	paths     bool
	budget    *ssaflow.SearchBudget
	storage   *heapmodel.Storage
	facts     map[*types.Func]Fact
}

// NewEngine builds a local-only engine; Analyzer additionally loads dependency facts.
func NewEngine() *Engine {
	engine := &Engine{}
	engine.summaries = engine.newSummaries(true)
	engine.linear = engine.newSummaries(false)
	return engine
}

// Fact publication cannot represent alternatives. Give it a separate, fixed
// cache policy so exported functions do not pay for paths only graph queries
// can use. A linear cutoff must not poison the richer query's cache.
func (engine *Engine) newSummaries(paths bool) *ssaflow.FunctionSummaries[Summary] {
	return ssaflow.NewFunctionSummaries(func(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
		builder := engine.query(budget)
		builder.paths = paths
		if !paths {
			builder.summaries = engine.linear
		}
		return builder.collect(function, false)
	}, unavailableSummary)
}

func unavailableSummary(reason ssaflow.SummaryUnavailable) Summary {
	switch reason {
	case ssaflow.SummaryRecursive:
		return Summary{Reason: ReasonRecursiveProtocol}
	case ssaflow.SummaryBudgetExhausted:
		return Summary{Reason: ReasonBudgetExhausted}
	case ssaflow.SummaryBodyUnavailable:
		return Summary{Reason: ReasonBodyUnavailable}
	}
	return Summary{Reason: ReasonEffectUnknown}
}

func (engine *Engine) query(budget *ssaflow.SearchBudget) *Engine {
	return &Engine{summaries: engine.summaries, facts: engine.facts, budget: budget, storage: heapmodel.NewStorage(budget), paths: true}
}

// Function summarizes a visible body, including bounded child templates.
func (engine *Engine) Function(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.summaries.Function(function, budget)
}

// Root collects a caller and at most maxWorkers children under one shared work budget.
func (engine *Engine) Root(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	query := engine.query(budget)
	return query.materialize(query.collect(function, true), function)
}

// AtCall binds complete local or imported effects to the caller's exact values.
func (engine *Engine) AtCall(call ssa.CallInstruction, budget *ssaflow.SearchBudget) Summary {
	// Analysis drivers may run sibling consumers in parallel. The cache and
	// its recursion guard form one transaction, so locking individual map
	// accesses would still let another query look like a recursive call.
	engine.mu.Lock()
	defer engine.mu.Unlock()
	query := engine.query(budget)
	return query.materialize(query.callSummary(call), call.Parent())
}

func (engine *Engine) collect(function *ssa.Function, root bool) Summary {
	engine.cutoff = nil
	result := engine.collectEffects(function, root)
	if engine.paths && result.Reason == ReasonControlFlowUnknown && function != nil && len(function.Blocks) != 0 {
		engine.cutoff = nil
		result = engine.collectCountedLoops(function, root)
	}
	if engine.paths && (result.Reason == ReasonBranchEffectsDiffer || result.Reason == ReasonBranchAlternatives) {
		engine.cutoff = nil
		result = engine.collectPaths(function, root)
	}
	result = finishCancellation(result)
	if result.Reason != ReasonNone && !result.AlternativesComplete && len(result.Paths) == 0 {
		result.cutoff = engine.cutoff
		if result.cutoff == nil {
			result.cutoff = &summaryCutoff{function: function}
		}
	}
	return result
}

func (engine *Engine) collectEffects(function *ssa.Function, root bool) Summary {
	if function == nil || len(function.Blocks) == 0 {
		return Summary{Reason: ReasonBodyUnavailable}
	}
	if !root {
		if result, handled := engine.collectSelectFunction(function); handled {
			return result
		}
	}
	if !straightLineBody(function) {
		return engine.collectBranches(function, root)
	}
	var result Summary
	if reason := engine.collectBlock(&result, function.Blocks[0], root); reason != ReasonNone {
		if reason == ReasonSelectAlternatives {
			result.Reason = reason
			return result
		}
		return Summary{Reason: reason}
	}
	if len(result.deferred) != 0 {
		return Summary{Reason: ReasonDeferredEffectsUnknown}
	}
	if result.hasWorkerAlternatives() {
		result.Reason = ReasonSelectAlternatives
		result.AlternativesComplete = true
	}
	return result
}

func (engine *Engine) collectBlock(result *Summary, block *ssa.BasicBlock, root bool) Reason {
	for _, instruction := range block.Instrs {
		if !engine.budget.Spend() {
			engine.recordCutoff(instruction, cutoffInstruction)
			return ReasonBudgetExhausted
		}
		if reason := engine.appendInstruction(result, instruction, root); reason != ReasonNone {
			return reason
		}
		if result.operationCount() > maxOperations {
			engine.recordCutoff(instruction, cutoffInstruction)
			return ReasonSummaryLimit
		}
	}
	return ReasonNone
}

func (engine *Engine) appendInstruction(result *Summary, instruction ssa.Instruction, root bool) Reason {
	engine.cutoff = nil
	reason := engine.instructionEffects(result, instruction, root)
	if reason != ReasonNone {
		engine.recordCutoff(instruction, cutoffInstruction)
	} else {
		engine.cutoff = nil
	}
	return reason
}

func (engine *Engine) instructionEffects(result *Summary, instruction ssa.Instruction, root bool) Reason {
	switch instruction := instruction.(type) {
	case *ssa.Send:
		// Passing a reference in a message can add a participant or expose
		// shared storage. Only scalar payloads belong to this first proof.
		if !scalarType(instruction.X.Type()) {
			return ReasonPayloadUnknown
		}
		return engine.appendOperation(result, Send, instruction.Chan, instruction.Pos())
	case *ssa.UnOp:
		return engine.appendUnOp(result, instruction)
	case *ssa.Select:
		return engine.appendSelect(result, instruction)
	case *ssa.Call:
		if isCancelConstructor(instruction.Common()) && !root {
			return ReasonLocalContextUnknown
		}
		return engine.appendCall(result, instruction)
	case *ssa.Defer:
		return engine.deferCompletion(result, instruction)
	case *ssa.RunDefers:
		for _, deferred := range slices.Backward(result.deferred) {
			result.Operations = append(result.Operations, deferred)
		}
		result.deferred = nil
	case *ssa.Go:
		return engine.appendGo(result, instruction)
	case *ssa.Extract:
		if call, ok := instruction.Tuple.(*ssa.Call); ok && isCancelConstructor(call.Common()) {
			return ReasonNone
		}
		return passiveInstruction(instruction, root)
	default:
		return passiveInstruction(instruction, root)
	}
	return ReasonNone
}

func (engine *Engine) appendUnOp(result *Summary, instruction *ssa.UnOp) Reason {
	if instruction.Op == token.ARROW {
		return engine.appendOperation(result, Receive, instruction.X, instruction.Pos())
	}
	if instruction.Op == token.MUL && ssaflow.ChannelType(instruction) {
		if path, exact := embeddedPath(instruction.X); exact && path.Depth > 0 {
			return ReasonNone
		}
	}
	if instruction.Op != token.MUL || !readableAddress(instruction.X) {
		return ReasonLoadUnknown
	}
	return ReasonNone
}

func (summary Summary) operationCount() int {
	count := len(summary.Operations) + len(summary.deferred) + len(summary.CancellationInputs)
	for _, worker := range summary.Workers {
		count += len(worker.Operations)
		for _, alternative := range worker.Alternatives {
			count += len(alternative)
		}
	}
	return count
}

func (engine *Engine) appendOperation(result *Summary, kind Kind, value ssa.Value, pos token.Pos) Reason {
	resource, ok := engine.reference(value)
	if !ok {
		return ReasonChannelIdentityUnknown
	}
	result.Operations = append(result.Operations, Operation{Kind: kind, Resource: resource, Source: pos, Site: pos})
	return ReasonNone
}

func passiveInstruction(instruction ssa.Instruction, root bool) Reason {
	if scalarInstruction(instruction) {
		return ReasonNone
	}
	switch instruction := instruction.(type) {
	case *ssa.If, *ssa.Jump:
		// The acyclic collector checks every successor and requires identical
		// ordered effects at joins and returns.
		return ReasonNone
	case *ssa.Phi:
		// Scalar values cannot change resource identity. Every incoming
		// computation is still checked by the instruction whitelist.
		if scalarType(instruction.Type()) {
			return ReasonNone
		}
	case *ssa.DebugRef, *ssa.Alloc, *ssa.MakeClosure:
		return ReasonNone
	case *ssa.FieldAddr:
		if _, exact := embeddedPath(instruction); exact && !synchronizationPointer(instruction.X.Type()) {
			return ReasonNone
		}
	case *ssa.Store:
		// A fresh group's zero state is part of the counter proof. Resetting
		// or copying it invalidates that proof, even through a local address.
		// Spilling a *pointer* to a primitive into a local closure cell does
		// not copy the primitive; binding later proves the cell is stable.
		if localSynchronizationPointerStore(instruction) ||
			localAddress(instruction.Addr) && !containsSynchronization(instruction.Val.Type()) &&
				!synchronizationPointer(instruction.Addr.Type()) {
			return ReasonNone
		}
	case *ssa.ChangeType:
		if ssaflow.ChannelType(instruction) {
			return ReasonNone
		}
	case *ssa.MakeInterface:
		// Boxing does not itself publish the value. Every subsequent use
		// must still resolve to a complete callee; opaque dispatch is unknown.
		return ReasonNone
	case *ssa.MakeChan:
		// Callee allocation sites cannot identify runtime instances across
		// separate calls. Only channels created by the root are admitted.
		if root {
			return ReasonNone
		}
	}
	// This whitelist is also the scope-completeness proof: no unmodelled
	// call, publication, launch, panic, or blocking action is skipped.
	return ReasonEffectUnknown
}

func localSynchronizationPointerStore(store *ssa.Store) bool {
	if !localAddress(store.Addr) {
		return false
	}
	pointer, ok := store.Addr.Type().Underlying().(*types.Pointer)
	return ok && synchronizationPointer(pointer.Elem())
}

// Empty protocol effects require positive evidence for every instruction, not
// just scalar arguments or a scalar result. Division and shifts can panic;
// unknown calls and reference-bearing returns must retain their usual bailout.
func scalarInstruction(instruction ssa.Instruction) bool {
	switch instruction := instruction.(type) {
	case *ssa.BinOp:
		if !scalarType(instruction.X.Type()) || !scalarType(instruction.Y.Type()) {
			return false
		}
		return slices.Contains([]token.Token{
			token.ADD, token.SUB, token.MUL, token.AND, token.OR, token.XOR, token.AND_NOT,
			token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ,
		}, instruction.Op)
	case *ssa.Return:
		for _, result := range instruction.Results {
			if !scalarType(result.Type()) {
				return false
			}
		}
		return true
	}
	return false
}

func scalarType(value types.Type) bool {
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
}
