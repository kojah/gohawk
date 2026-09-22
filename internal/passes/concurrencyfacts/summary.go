// Package concurrencyfacts composes complete, bounded synchronization effects.
// It provides ordered evidence, not deadlock policy or schedule exploration.
package concurrencyfacts

import (
	"go/token"
	"go/types"
	"slices"
	"sync"

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
)

// Reference names an exact resource or a symbolic captured cell.
type Reference struct {
	Value    ssa.Value
	Indirect bool
}

// Operation retains execution order and source/call-site provenance.
type Operation struct {
	Kind     Kind
	Resource Reference
	Source   token.Pos
	Site     token.Pos
}

// Summary is complete only when Reason is empty. Returned slices are immutable.
// Worker and Prefix describe the sole launch allowed in a root query.
type Summary struct {
	Operations []Operation
	deferred   []Operation
	Worker     []Operation
	Spawn      *ssa.Go
	Prefix     int
	Reason     string
}

// Engine caches evidence for one package. Concurrent consumers are serialized
// at the public query boundary; each query must supply its own work budget.
type Engine struct {
	mu        sync.Mutex
	summaries *ssaflow.FunctionSummaries[Summary]
	budget    *ssaflow.SearchBudget
	storage   *ssaflow.Storage
	facts     map[*types.Func]Fact
}

// NewEngine builds a local-only engine; Analyzer additionally loads dependency facts.
func NewEngine() *Engine {
	engine := &Engine{}
	engine.summaries = ssaflow.NewFunctionSummaries(func(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
		builder := engine.query(budget)
		return builder.collect(function, false)
	}, unavailableSummary)
	return engine
}

func unavailableSummary(reason ssaflow.SummaryUnavailable) Summary {
	switch reason {
	case ssaflow.SummaryRecursive:
		return Summary{Reason: "recursive-protocol"}
	case ssaflow.SummaryBudgetExhausted:
		return Summary{Reason: "protocol-budget-exhausted"}
	case ssaflow.SummaryBodyUnavailable:
		return Summary{Reason: "protocol-body-unavailable"}
	}
	return Summary{Reason: "protocol-effect-unknown"}
}

func (engine *Engine) query(budget *ssaflow.SearchBudget) *Engine {
	return &Engine{summaries: engine.summaries, facts: engine.facts, budget: budget, storage: ssaflow.NewStorage(budget)}
}

// Function summarizes a visible body without allowing nested launches.
func (engine *Engine) Function(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.summaries.Function(function, budget)
}

// Root collects a caller and at most one worker under one shared work budget.
func (engine *Engine) Root(function *ssa.Function, budget *ssaflow.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.query(budget).collect(function, true)
}

// AtCall binds complete local or imported effects to the caller's exact values.
func (engine *Engine) AtCall(call ssa.CallInstruction, budget *ssaflow.SearchBudget) Summary {
	// Analysis drivers may run sibling consumers in parallel. The cache and
	// its recursion guard form one transaction, so locking individual map
	// accesses would still let another query look like a recursive call.
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.query(budget).callSummary(call)
}

func (engine *Engine) collect(function *ssa.Function, root bool) Summary {
	if function == nil || len(function.Blocks) == 0 {
		return Summary{Reason: "protocol-body-unavailable"}
	}
	if !straightLineBody(function) {
		return engine.collectBranches(function, root)
	}
	var result Summary
	if reason := engine.collectBlock(&result, function.Blocks[0], root); reason != "" {
		return Summary{Reason: reason}
	}
	if len(result.deferred) != 0 {
		return Summary{Reason: "protocol-deferred-effects-unknown"}
	}
	return result
}

func (engine *Engine) collectBlock(result *Summary, block *ssa.BasicBlock, root bool) string {
	for _, instruction := range block.Instrs {
		if !engine.budget.Spend() {
			return "protocol-budget-exhausted"
		}
		if reason := engine.appendInstruction(result, instruction, root); reason != "" {
			return reason
		}
		if len(result.Operations)+len(result.Worker)+len(result.deferred) > maxOperations {
			return "protocol-summary-limit"
		}
	}
	return ""
}

func (engine *Engine) appendInstruction(result *Summary, instruction ssa.Instruction, root bool) string {
	switch instruction := instruction.(type) {
	case *ssa.Send:
		// Passing a reference in a message can add a participant or expose
		// shared storage. Only scalar payloads belong to this first proof.
		if !scalarType(instruction.X.Type()) {
			return "protocol-payload-unknown"
		}
		return engine.appendOperation(result, Send, instruction.Chan, instruction.Pos())
	case *ssa.UnOp:
		if instruction.Op == token.ARROW {
			return engine.appendOperation(result, Receive, instruction.X, instruction.Pos())
		}
		if instruction.Op != token.MUL || !readableAddress(instruction.X) {
			return "protocol-load-unknown"
		}
	case *ssa.Call:
		called := engine.callSummary(instruction)
		result.Operations = append(result.Operations, called.Operations...)
		return called.Reason
	case *ssa.Defer:
		return engine.deferCompletion(result, instruction)
	case *ssa.RunDefers:
		for _, deferred := range slices.Backward(result.deferred) {
			result.Operations = append(result.Operations, deferred)
		}
		result.deferred = nil
	case *ssa.Go:
		if !root || result.Spawn != nil {
			return "protocol-participants-unknown"
		}
		called := engine.instantiate(instruction)
		result.Spawn, result.Prefix, result.Worker = instruction, len(result.Operations), called.Operations
		return called.Reason
	default:
		return passiveInstruction(instruction, root)
	}
	return ""
}

func (engine *Engine) appendOperation(result *Summary, kind Kind, value ssa.Value, pos token.Pos) string {
	resource, ok := engine.reference(value)
	if !ok {
		return "protocol-channel-identity-unknown"
	}
	result.Operations = append(result.Operations, Operation{Kind: kind, Resource: resource, Source: pos, Site: pos})
	return ""
}

func passiveInstruction(instruction ssa.Instruction, root bool) string {
	if scalarInstruction(instruction) {
		return ""
	}
	switch instruction := instruction.(type) {
	case *ssa.If, *ssa.Jump:
		// The acyclic collector checks every successor and requires identical
		// ordered effects at joins and returns.
		return ""
	case *ssa.Phi:
		// Scalar values cannot change resource identity. Every incoming
		// computation is still checked by the instruction whitelist.
		if scalarType(instruction.Type()) {
			return ""
		}
	case *ssa.DebugRef, *ssa.Alloc, *ssa.MakeClosure:
		return ""
	case *ssa.FieldAddr:
		if _, exact := embeddedPath(instruction); exact && !synchronizationPointer(instruction.X.Type()) {
			return ""
		}
	case *ssa.Store:
		// A fresh group's zero state is part of the counter proof. Resetting
		// or copying it invalidates that proof, even through a local address.
		if localAddress(instruction.Addr) && !containsSynchronization(instruction.Val.Type()) &&
			!synchronizationPointer(instruction.Addr.Type()) {
			return ""
		}
	case *ssa.ChangeType:
		if ssaflow.ChannelType(instruction) {
			return ""
		}
	case *ssa.MakeInterface:
		// A concrete Mutex may be bound to NewCond's Locker. Consumers of
		// the interface still need complete call summaries; publication is opaque.
		if MutexPointer(instruction.X.Type()) {
			return ""
		}
	case *ssa.MakeChan:
		// Callee allocation sites cannot identify runtime instances across
		// separate calls. Only channels created by the root are admitted.
		if root {
			return ""
		}
	}
	// This whitelist is also the scope-completeness proof: no unmodelled
	// call, publication, launch, panic, or blocking action is skipped.
	return "protocol-effect-unknown"
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
