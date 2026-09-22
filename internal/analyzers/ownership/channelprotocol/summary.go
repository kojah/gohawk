// Package channelprotocol proves bounded channel waiting cycles by composing
// local function summaries. It never enumerates schedules. Unknown effects,
// control flow, or participation prevent a diagnostic.
package channelprotocol

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

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
)

type channelReference struct {
	value    ssa.Value
	indirect bool
}

type operation struct {
	kind    operationKind
	channel channelReference
	source  token.Pos
	site    token.Pos
}

type summary struct {
	operations []operation
	worker     []operation
	spawn      *ssa.Go
	prefix     int
	reason     string
}

type summaryEngine struct {
	memo    *ssaflow.CallGraphMemo[*ssa.Function, summary]
	budget  *ssaflow.SearchBudget
	storage *ssaflow.Storage
}

func newSummaryEngine() *summaryEngine {
	return &summaryEngine{memo: ssaflow.NewCallGraphMemo[*ssa.Function, summary]()}
}

func (engine *summaryEngine) begin(limit int) {
	engine.budget = ssaflow.NewSearchBudget(limit)
	engine.storage = ssaflow.NewStorage(engine.budget)
}

func (engine *summaryEngine) summarize(function *ssa.Function) summary {
	return engine.memo.Answer(function, func() summary {
		if !engine.memo.Enter(function) {
			return summary{reason: "recursive-protocol"}
		}
		defer engine.memo.Leave(function)
		result := engine.collect(function, false)
		if engine.budget.Exhausted() {
			engine.memo.Cut()
			return summary{reason: "protocol-budget-exhausted"}
		}
		return result
	})
}

func (engine *summaryEngine) collect(function *ssa.Function, root bool) summary {
	if function == nil || len(function.Blocks) == 0 {
		return summary{reason: "protocol-body-unavailable"}
	}
	if len(function.Blocks) != 1 {
		return summary{reason: "protocol-control-flow-unknown"}
	}
	var result summary
	for _, instruction := range function.Blocks[0].Instrs {
		if !engine.budget.Spend() {
			engine.memo.Cut()
			return summary{reason: "protocol-budget-exhausted"}
		}
		if reason := engine.appendInstruction(&result, instruction, root); reason != "" {
			return summary{reason: reason}
		}
		if len(result.operations)+len(result.worker) > maxOperations {
			return summary{reason: "protocol-summary-limit"}
		}
	}
	return result
}

func (engine *summaryEngine) appendInstruction(result *summary, instruction ssa.Instruction, root bool) string {
	switch instruction := instruction.(type) {
	case *ssa.Send:
		// Passing a reference in a message can add a participant or expose
		// shared storage. Only scalar payloads belong to this first proof.
		if _, ok := instruction.X.Type().Underlying().(*types.Basic); !ok {
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
		if ssaflow.CallMatchesSymbol(instruction.Common(), syntax.Builtin("close")) {
			return engine.appendOperation(result, closeOperation, instruction.Common().Args[0], instruction.Pos())
		}
		called := engine.instantiate(instruction)
		result.operations = append(result.operations, called.operations...)
		return called.reason
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
	channel, ok := engine.reference(value)
	if !ok {
		return "protocol-channel-identity-unknown"
	}
	result.operations = append(result.operations, operation{kind: kind, channel: channel, source: pos, site: pos})
	return ""
}

func passiveInstruction(instruction ssa.Instruction, root bool) string {
	switch instruction := instruction.(type) {
	case *ssa.DebugRef, *ssa.Alloc, *ssa.MakeClosure:
		return ""
	case *ssa.FieldAddr:
		if localAddress(instruction.X) {
			return ""
		}
	case *ssa.Store:
		if localAddress(instruction.Addr) {
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
	case *ssa.Return:
		if len(instruction.Results) == 0 {
			return ""
		}
	}
	// This whitelist is also the scope-completeness proof: no unmodelled
	// call, publication, launch, defer, panic, or blocking action is skipped.
	return "protocol-effect-unknown"
}
