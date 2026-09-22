// Package channelprotocol proves bounded channel waiting cycles by composing
// local function summaries. It never enumerates schedules. Unknown effects,
// control flow, or participation prevent a diagnostic.
package channelprotocol

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Summaries preserve a bounded ordered sequence over symbolic parameters and
// captures. They are local to this pass: lifecycle bitsets cannot encode this
// relation, and unavailable dependency bodies remain unknown. We deliberately
// do not merge branches into a sequence or truncate an overflowing summary.
const maxOperations = 32

type operationKind uint8

const (
	sendOperation operationKind = iota
	receiveOperation
	closeOperation
	groupAddOperation
	groupDoneOperation
	groupWaitOperation
)

type resourceReference struct {
	value    ssa.Value
	indirect bool
}

type operation struct {
	kind     operationKind
	resource resourceReference
	source   token.Pos
	site     token.Pos
}

type summary struct {
	operations []operation
	deferred   []operation
	worker     []operation
	spawn      *ssa.Go
	prefix     int
	reason     string
}

type summaryEngine struct {
	summaries *ssaflow.FunctionSummaries[summary]
	budget    *ssaflow.SearchBudget
	storage   *ssaflow.Storage
}

func newSummaryEngine() *summaryEngine {
	engine := &summaryEngine{}
	engine.summaries = ssaflow.NewFunctionSummaries(func(function *ssa.Function, budget *ssaflow.SearchBudget) summary {
		builder := &summaryEngine{summaries: engine.summaries, budget: budget, storage: ssaflow.NewStorage(budget)}
		return builder.collect(function, false)
	}, unavailableSummary)
	return engine
}

func unavailableSummary(reason ssaflow.SummaryUnavailable) summary {
	switch reason {
	case ssaflow.SummaryRecursive:
		return summary{reason: "recursive-protocol"}
	case ssaflow.SummaryBudgetExhausted:
		return summary{reason: "protocol-budget-exhausted"}
	case ssaflow.SummaryBodyUnavailable:
		return summary{reason: "protocol-body-unavailable"}
	}
	return summary{reason: "protocol-effect-unknown"}
}

func (engine *summaryEngine) begin(limit int) {
	engine.budget = ssaflow.NewSearchBudget(limit)
	engine.storage = ssaflow.NewStorage(engine.budget)
}

func (engine *summaryEngine) collect(function *ssa.Function, root bool) summary {
	if function == nil || len(function.Blocks) == 0 {
		return summary{reason: "protocol-body-unavailable"}
	}
	if !straightLineBody(function) {
		return summary{reason: "protocol-control-flow-unknown"}
	}
	var result summary
	for _, instruction := range function.Blocks[0].Instrs {
		if !engine.budget.Spend() {
			return summary{reason: "protocol-budget-exhausted"}
		}
		if reason := engine.appendInstruction(&result, instruction, root); reason != "" {
			return summary{reason: reason}
		}
		if len(result.operations)+len(result.worker)+len(result.deferred) > maxOperations {
			return summary{reason: "protocol-summary-limit"}
		}
	}
	if len(result.deferred) != 0 {
		return summary{reason: "protocol-deferred-effects-unknown"}
	}
	return result
}

func (engine *summaryEngine) appendInstruction(result *summary, instruction ssa.Instruction, root bool) string {
	switch instruction := instruction.(type) {
	case *ssa.Send:
		// Passing a reference in a message can add a participant or expose
		// shared storage. Only scalar payloads belong to this first proof.
		if !scalarType(instruction.X.Type()) {
			return "protocol-payload-unknown"
		}
		return engine.appendOperation(result, sendOperation, instruction.Chan, instruction.Pos())
	case *ssa.UnOp:
		if instruction.Op == token.ARROW {
			return engine.appendOperation(result, receiveOperation, instruction.X, instruction.Pos())
		}
		if instruction.Op != token.MUL || !readableAddress(instruction.X) {
			return "protocol-load-unknown"
		}
	case *ssa.Call:
		called := engine.callSummary(instruction)
		result.operations = append(result.operations, called.operations...)
		return called.reason
	case *ssa.Defer:
		return engine.deferCompletion(result, instruction)
	case *ssa.RunDefers:
		for _, deferred := range slices.Backward(result.deferred) {
			result.operations = append(result.operations, deferred)
		}
		result.deferred = nil
	case *ssa.Go:
		if !root || result.spawn != nil {
			return "protocol-participants-unknown"
		}
		called := engine.instantiate(instruction)
		result.spawn, result.prefix, result.worker = instruction, len(result.operations), called.operations
		return called.reason
	default:
		return passiveInstruction(instruction, root)
	}
	return ""
}

func (engine *summaryEngine) appendOperation(result *summary, kind operationKind, value ssa.Value, pos token.Pos) string {
	resource, ok := engine.reference(value)
	if !ok {
		return "protocol-channel-identity-unknown"
	}
	result.operations = append(result.operations, operation{kind: kind, resource: resource, source: pos, site: pos})
	return ""
}

func passiveInstruction(instruction ssa.Instruction, root bool) string {
	if scalarInstruction(instruction) {
		return ""
	}
	switch instruction := instruction.(type) {
	case *ssa.DebugRef, *ssa.Alloc, *ssa.MakeClosure:
		return ""
	case *ssa.FieldAddr:
		if localAddress(instruction.X) && !waitGroupPointer(instruction.X.Type()) {
			return ""
		}
	case *ssa.Store:
		// A fresh group's zero state is part of the counter proof. Resetting
		// or copying it invalidates that proof, even through a local address.
		if localAddress(instruction.Addr) && !waitGroupPointer(instruction.Addr.Type()) {
			return ""
		}
	case *ssa.ChangeType:
		if ssaflow.ChannelType(instruction) {
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
