// Package concurrencyfacts composes complete, bounded synchronization effects.
// It provides ordered evidence, not deadlock policy or schedule exploration.
package concurrencyfacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
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
	// Invoke calls a function-typed input, or a method of an interface input,
	// at this point. It is a hole, not an effect: binding replaces it with the
	// supplied function's or concrete method's effects, and a summary that
	// still contains one is incomplete (see callbacks.go).
	Invoke
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
	// Method is the interface method an Invoke hole calls, or nil when the
	// hole calls a function value.
	Method *types.Func
	// Alternates are the sources of the same operation on other branches that
	// folded into this one because their ordered effects were equal. Source
	// stays the first branch's position. Local diagnostic metadata only.
	Alternates []token.Pos
}

// Condition is one branch choice that selects a path alternative. Without
// Compared it says Value, a Boolean, is Holds. With Compared it says whether
// Value == Compared is Holds; a != test is stored as == with Holds inverted,
// so the comparison can be rebound to a caller's value. Context lists the
// call sites the alternative was bound through, innermost first, so one
// helper called twice keeps two separate conditions. Conditions are evidence
// for feasibility queries only; they never make a summary complete.
type Condition struct {
	Value    ssa.Value
	Compared *ssa.Const
	Holds    bool
	Context  []token.Pos
	// Implied marks a fact that follows from the path, such as what a helper
	// returned, rather than a choice: it can contradict other conditions but
	// determines its value instead of adding an independent input.
	Implied bool
}

// Returned says result Index is Constant on a path, or, with no Constant,
// that it cannot be nil. Results from calls, loads, and parameters are not
// recorded: nothing structural says what they are.
type Returned struct {
	Index    int
	Constant *ssa.Const
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
	Paths []Summary
	// Conditions are the branch choices that select this summary when it is
	// one of Paths. A select arm is the runtime's choice and adds none.
	Conditions []Condition
	// Returned is what this path returns where that is structurally certain,
	// so a caller can relate its own tests of the result to this path.
	Returned   []Returned
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
	// AlternativeConditions parallels Alternatives for branch paths: the
	// worker's own branch choices that select each alternative.
	AlternativeConditions [][]Condition
	// Replicated marks one representative of the identical workers a worker
	// pool launches an unknown number of times; see replicated_workers.go.
	Replicated bool
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
	case summary.Reason != ReasonNone || len(summary.Paths) != 0 || !summary.CancellationBound() || !summary.CallbacksBound() ||
		summary.hasReplicatedWorkers():
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

// A closed interface receiver can reuse direct summaries without changing
// argument positions: invocation mode stores the receiver in Value, whereas
// direct mode uses Args[0]. Never guess a target from the method name, the set
// of implementations in a package, or one possible receiver at a phi.
func (engine *Engine) resolvedCommon(instruction ssa.CallInstruction) *ssa.CallCommon {
	common := instruction.Common()
	if !common.IsInvoke() {
		return common
	}
	dispatch := ssacall.ResolveInterfaceDispatch(common, instruction.Parent().Prog, engine.budget)
	if !dispatch.Proven() {
		return common
	}
	bound := *common
	bound.Method, bound.Value = nil, dispatch.Function
	bound.Args = append([]ssa.Value{dispatch.Receiver}, common.Args...)
	return &bound
}
