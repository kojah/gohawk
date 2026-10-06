package processownership

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Synchronous Read/Write/Close on a known command pipe consumes IO locally;
// it neither hands on a process handle nor returns one in its result. Returning
// the pipe itself or passing it to other code retains the ordinary ownership
// question. This applies the unused-command boundary without proving reaping.
// https://github.com/jayu/rev-dep/blob/8a2fdb0927e2fc9b2a5b178c94f55d1887659152/internal/telemetry/telemetry.go#L75-L103
func commandPipeOperation(common *ssa.CallCommon) bool {
	if common == nil {
		return false
	}
	pipe, index, called := ssaflow.CallResultSource(ssaflow.CallReceiver(common))
	if !called || index != 0 || !ssaflow.CallMatchesAnySymbol(pipe.Common(),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "StdinPipe"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "StdoutPipe"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "StderrPipe"})) {
		return false
	}
	return ssaflow.CallMatchesAnySymbol(common,
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "io", Receiver: "WriteCloser", Name: "Write"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "io", Receiver: "WriteCloser", Name: "Close"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "io", Receiver: "ReadCloser", Name: "Read"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "io", Receiver: "ReadCloser", Name: "Close"}))
}

func osProcessDerivedFromCommand(value, command ssa.Value) bool {
	if value == nil || value.Type() == nil {
		return false
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "os", "Process") && heapmodel.ValueDerivesFrom(value, command)
}

// returnsProcessHandle reports whether a return hands the caller the exact
// os.Process projected from the started command.
func returnsProcessHandle(returned *ssa.Return, command ssa.Value) bool {
	for _, result := range returned.Results {
		if osProcessDerivedFromCommand(result, command) {
			return true
		}
	}
	return false
}

// commandForms are the wrappers a command keeps its provenance through.
const commandForms = ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface

func commandReturnedByHelper(command ssa.Value) bool {
	// Commands produced by a helper may carry a lifecycle contract the caller
	// cannot see. Track only transparent SSA wrappers and merges; a direct
	// os/exec constructor remains locally owned and must still be waited for.
	return ssaflow.NewReachingWalk(commandForms).Any(command, commandReturnedByHelperLeaf)
}

func commandReturnedByHelperLeaf(walk ssaflow.ReachingWalk, command ssa.Value) bool {
	switch typed := command.(type) {
	case *ssa.Extract:
		// Tuple-returning factories carry the same uncertain ownership as
		// single-result factories. Only follow an actual call result, not an
		// arbitrary tuple-producing operation.
		// https://github.com/mutagen-io/mutagen/blob/6ccfeaaf4dfd261e59ef9aac56e3c157b62e605b/pkg/agent/dial.go#L80-L118
		_, called := typed.Tuple.(*ssa.Call)
		return called && walk.Any(typed.Tuple, commandReturnedByHelperLeaf)
	case *ssa.Call:
		return !ssaflow.CallMatchesAnySymbol(
			typed.Common(),
			syntax.PackageFunction("os/exec", "Command"),
			syntax.PackageFunction("os/exec", "CommandContext"),
		)
	case *ssa.UnOp:
		if typed.X.Referrers() == nil {
			return false
		}
		for _, reference := range *typed.X.Referrers() {
			store, ok := reference.(*ssa.Store)
			if ok && store.Addr == typed.X && walk.Any(store.Val, commandReturnedByHelperLeaf) {
				return true
			}
		}
	}
	return false
}

func parameterValues(parameters []*ssa.Parameter) []ssa.Value {
	values := make([]ssa.Value, len(parameters))
	for index, parameter := range parameters {
		values[index] = parameter
	}
	return values
}

func execCommandValue(value ssa.Value) bool {
	if value == nil {
		return false
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "os/exec", "Cmd")
}

func waitsForCommand(instruction ssa.Instruction, command ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	receiver := ssaflow.CallReceiver(common)
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Wait"})) {
		// A closure-local FreeVar is the mapped capture cell, not the command
		// value. Its caller maps the captured command into this frame.
		if _, captured := command.(*ssa.FreeVar); captured {
			return heapmodel.ValueDerivesFrom(receiver, command)
		}
		return heapmodel.NewStorage(nil).Same(receiver, command).Proven()
	}
	// Waiting through cmd.Process reaps the same operating-system child. Mache
	// uses the lower-level handle after signaling an entire process group:
	// https://github.com/agentic-research/mache/blob/ccaf44c3688c12324af57747b6fb0c6a33ca93e0/internal/leyline/procgroup_unix_test.go#L30-L51
	return ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os", Receiver: "Process", Name: "Wait"})) &&
		osProcessDerivedFromCommand(receiver, command)
}

// A command created in one branch may merge with nil from branches that never
// started a child. On the exact successful Start edge the merge is that command,
// not a possible alias. A cyclic merge could later select a different value and
// is deliberately excluded. The caller starts its ordinary non-nil flow at the
// phi, before any executable instruction in this success block.
// https://github.com/threatexpert/gonc/blob/e14bc6b97efc2150c4e0bbb2bdc89f8548e28fa6/apps/nc.go#L3171-L3327
func successfulCommandMerge(start *ssa.Call, command ssa.Value) *ssa.Phi {
	for _, successor := range start.Block().Succs {
		success, known := ssaflow.SuccessBranch(start.Block(), successor, start)
		if !known || !success || ssaflow.BlockInCycle(successor) {
			continue
		}
		for _, instruction := range successor.Instrs {
			phi, ok := instruction.(*ssa.Phi)
			if !ok {
				break
			}
			for predecessor, incoming := range ssaflow.PhiIncoming(phi) {
				if predecessor == start.Block() && incoming == command {
					return phi
				}
			}
		}
	}
	return nil
}

type processGuardProof struct {
	State  proofs.EvidenceState
	Reason processReason
	NonNil ssa.Value
}

// Successful Start guarantees Process is non-nil at an immediate defensive
// guard. Later stores or opaque calls can invalidate that fact, so require
// mutation-free adjacent blocks and fix only this load's value in the flow.
// A return after the Release branch can merge with the impossible nil branch;
// pruning that edge avoids a second, return-specific feasibility proof.
// https://github.com/TencentCloud/tencentmeeting-cli/blob/e631b355da2b001d24b82f453b65d96f39c59865/internal/event/spawner/spawner.go#L113-L123
// https://github.com/stacktower-io/stacktower/blob/69ff07430089898cc79af381f6e0c3a927a7d149/internal/cli/auth_device.go#L118-L124
func proveImmediateProcessGuard(start *ssa.Call, command ssa.Value) processGuardProof {
	if ssaflow.InstructionIndex(start) != len(start.Block().Instrs)-3 {
		return processGuardProof{State: proofs.EvidenceDisproven}
	}
	for _, guard := range start.Block().Succs {
		if len(guard.Preds) != 1 || guard.Preds[0] != start.Block() || len(guard.Instrs) != 4 {
			continue
		}
		if success, known := ssaflow.SuccessBranch(start.Block(), guard, start); !known || !success {
			continue
		}
		comparison := immediateProcessNilComparison(guard, command)
		if comparison == nil || comparison.Op != token.EQL && comparison.Op != token.NEQ {
			continue
		}
		value := comparison.X
		if ssaflow.DefinitelyNil(value) {
			value = comparison.Y
		}
		return processGuardProof{State: proofs.EvidenceProven, Reason: reasonSuccessfulStartProcessNonNil, NonNil: value}
	}
	return processGuardProof{State: proofs.EvidenceDisproven}
}

// immediateProcessNilComparison matches a block that only reads the command's
// Process and branches on its nilness, with no intervening call or mutation.
func immediateProcessNilComparison(guard *ssa.BasicBlock, command ssa.Value) *ssa.BinOp {
	field, fieldOK := guard.Instrs[0].(*ssa.FieldAddr)
	load, loadOK := guard.Instrs[1].(*ssa.UnOp)
	comparison, comparisonOK := guard.Instrs[2].(*ssa.BinOp)
	branch, branchOK := guard.Instrs[3].(*ssa.If)
	if !fieldOK || !loadOK || !comparisonOK || !branchOK || len(guard.Succs) != 2 {
		return nil
	}
	if !heapmodel.NewStorage(nil).Same(field.X, command).Proven() || load.X != field || load.Op != token.MUL ||
		!osProcessDerivedFromCommand(load, command) || branch.Cond != comparison {
		return nil
	}
	comparesProcessNil := comparison.X == load && ssaflow.DefinitelyNil(comparison.Y) ||
		comparison.Y == load && ssaflow.DefinitelyNil(comparison.X)
	if !comparesProcessNil {
		return nil
	}
	return comparison
}

func startFailureReturn(returned *ssa.Return, start *ssa.Call) bool {
	// Wait is not required on the path where Start itself failed. Accept that
	// exception only when SSA branch evidence separates failure from every path
	// that can reach the same return.
	if returned.Block() == start.Block() {
		return false
	}
	for _, predecessor := range returned.Block().Preds {
		if success, known := ssaflow.SuccessBranch(predecessor, returned.Block(), start); known {
			return !success
		}
	}
	for _, successor := range start.Block().Succs {
		success, known := ssaflow.SuccessBranch(start.Block(), successor, start)
		if !known || success {
			continue
		}
		return ssaflow.BlockReachable(successor, returned.Block()) && !successBranchReaches(start, returned.Block())
	}
	return false
}

func successBranchReaches(start *ssa.Call, target *ssa.BasicBlock) bool {
	for _, successor := range start.Block().Succs {
		if success, known := ssaflow.SuccessBranch(start.Block(), successor, start); known && success {
			return ssaflow.BlockReachable(successor, target)
		}
	}
	return false
}
