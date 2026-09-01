// Package cancellationownership implements the cancellationownership gohawk analyzer.
package cancellationownership

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/analysispasses/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "cancellationownership",
		Doc:      "checks context and signal-derived cancellation functions are called on every return path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, lifecyclefacts.Analyzer},
		Run:      runCancellationOwnership,
	}
}

func runCancellationOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				contract, ok := cancellationContractFor(call.Common())
				if !ok {
					continue
				}
				cancel := ssautil.CallResult(call, contract.result)
				if cancel == nil {
					continue
				}
				// Cleanup need not occur in this function: returning the cancel,
				// storing it in an owner, or installing a callback that invokes it
				// transfers the obligation. Reassigned captured locals are included;
				// Prometheus installs its current cancel in a scraper callback:
				// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/scrape/scrape_test.go#L1294-L1315
				leaks := ssautil.UnownedReturnAssumingNonNil(call, cancel, func(candidate ssa.Instruction) bool {
					return callsCancel(pass, candidate, cancel)
				}, func(returned *ssa.Return) bool {
					return ssautil.ReturnSameValue(returned, cancel) || ssautil.ReturnedValueOwnsValue(returned, cancel)
				})
				emitCancellationDecision(pass, function, call, contract, leaks)
				if leaks {
					source := analysisutil.SourceRange(pass, call.Pos())
					check.Report(pass, check.CancellationRelease, analysis.Diagnostic{
						Pos: source.Pos(),
						End: source.End(),
						Message: "cancel function from " + analysisutil.ShortPackageName(
							contract.packagePath,
						) + "." + contract.name + " is not called on every return path",
						SuggestedFixes: cancellationFix(pass, source.Pos(), contract.name),
					})
				}
			}
		}
	}
	return nil, nil
}

func emitCancellationDecision(pass *analysis.Pass, function *ssa.Function, call *ssa.Call, contract cancellationContract, leaks bool) {
	checkID := string(check.CancellationRelease)
	if !analysisTrace.Enabled("cancellationownership", checkID) {
		return
	}
	outcome, reason := analysisTrace.OutcomeAccepted, "release-proven"
	if leaks {
		outcome, reason = analysisTrace.OutcomeRejected, "unowned-return"
	}
	analysisTrace.Emit(
		pass,
		analysisTrace.Event{
			Analyzer: "cancellationownership",
			Check:    checkID,
			Phase:    "decision",
			Reason:   reason,
			Outcome:  outcome,
			Pos:      call.Pos(),
			Function: function.String(),
			Details:  map[string]string{"constructor": contract.packagePath + "." + contract.name},
		},
	)
}

type cancellationContract struct {
	packagePath string
	name        string
	result      int
}

func cancellationContractFor(common *ssa.CallCommon) (cancellationContract, bool) {
	packagePath, name := ssautil.CallPackage(common), ssautil.CallName(common)
	if packagePath == "context" && strings.HasPrefix(name, "With") {
		return cancellationContract{packagePath: packagePath, name: name, result: 1}, true
	}
	if packagePath == "os/signal" && name == "NotifyContext" {
		return cancellationContract{packagePath: packagePath, name: name, result: 1}, true
	}
	return cancellationContract{}, false
}

func cancellationFix(pass *analysis.Pass, callPosition token.Pos, constructor string) []analysis.SuggestedFix {
	for _, file := range pass.Files {
		var fix []analysis.SuggestedFix
		ast.Inspect(file, func(node ast.Node) bool {
			block, ok := node.(*ast.BlockStmt)
			if !ok {
				return fix == nil
			}
			for _, statement := range block.List {
				assignment, assignmentOK := statement.(*ast.AssignStmt)
				if !assignmentOK || len(assignment.Rhs) != 1 || len(assignment.Lhs) < 2 {
					continue
				}
				call, callOK := assignment.Rhs[0].(*ast.CallExpr)
				cancel, cancelOK := assignment.Lhs[1].(*ast.Ident)
				if !callOK || !cancelOK || call.Pos() != callPosition || cancel.Name == "_" {
					continue
				}
				insertAt, ok := nextLineStart(pass.Fset, assignment)
				if !ok {
					continue
				}
				fix = []analysis.SuggestedFix{{
					Message: "Defer " + cancel.Name + " immediately after creation",
					TextEdits: []analysis.TextEdit{{
						Pos:     insertAt,
						NewText: []byte("\tdefer " + cancelInvocation(cancel.Name, constructor) + "\n"),
					}},
				}}
				return false
			}
			return true
		})
		if fix != nil {
			return fix
		}
	}
	return nil
}

func nextLineStart(files *token.FileSet, node ast.Node) (token.Pos, bool) {
	file := files.File(node.End())
	if file == nil {
		return token.NoPos, false
	}
	line := file.Line(node.End())
	if line >= file.LineCount() {
		return token.NoPos, false
	}
	return file.LineStart(line + 1), true
}

func cancelInvocation(name, constructor string) string {
	if constructor == "WithCancelCause" {
		return name + "(nil)"
	}
	return name + "()"
}

func callsCancel(pass *analysis.Pass, instruction ssa.Instruction, cancel ssa.Value) bool {
	// A helper may settle an obligation without a naming convention. Require
	// invocation on every normal helper return; process-tree tests use this to
	// centralize cancellation and process cleanup together:
	// https://github.com/applicate2628/mcp-local-hub/blob/73fbad63f7f9f0b24caef2239256f53b70a74061/internal/vcpkgmcp/reversedepgraph/process_tree_test.go#L46
	// Cross-package helpers use exported lifecycle facts; the deferred boundary
	// remains for wrappers which delegate to an interface cleanup method.
	// Cerberus delegates qcancel through a deferred CloseCursor call:
	// https://github.com/tsouza/cerberus/blob/4d90ae7ec1061a357964795d5718ef0a40d06139/internal/solver/executor.go#L432
	if instructionSettlesCancellation(instruction, cancel) ||
		callTakesCancellationOwnership(pass, instruction, cancel) {
		return true
	}
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if ssautil.SameValue(common.Value, cancel) {
		return true
	}
	callbackRegistrar := ssautil.HasLibraryContract(common, ssautil.ContractTestingCleanup) ||
		ssautil.HasLibraryContract(common, ssautil.ContractAfterFunc) ||
		ssautil.HasLibraryContract(common, ssautil.ContractDeferredCleanup)
	for _, argument := range common.Args {
		// Registrars accept either a wrapper callback or the cancellation
		// function itself. Vekil installs cancel directly with time.AfterFunc:
		// https://github.com/sozercan/vekil/blob/842f12f7875143274378fcbb80d411295edf3d28/launch/runtime_test.go#L379
		if callbackRegistrar && (ssautil.SameValue(argument, cancel) || ssautil.ValueCallsValue(argument, cancel)) {
			return true
		}
	}
	return false
}

func instructionSettlesCancellation(instruction ssa.Instruction, cancel ssa.Value) bool {
	return ssautil.ClosureCallsValue(instruction, cancel) ||
		ssautil.DeferredClosureInvokesArgumentOnEveryReturn(instruction, cancel) ||
		ssautil.DeferredClosurePassesValueToNamedCall(
			instruction,
			cancel,
			"cancel",
			"cleanup",
			"close",
			"stop",
			"teardown",
		) ||
		ssautil.ClosureOwnsValue(instruction, cancel) ||
		ssautil.StoresValueInField(instruction, cancel) ||
		ssautil.StoresValueThroughExternalFieldPointer(instruction, cancel) ||
		ssautil.StoresValueInGlobal(instruction, cancel) ||
		ssautil.StoresOwnerOfValueInField(instruction, cancel) ||
		ssautil.StoresValueInOwnedMap(instruction, cancel) ||
		atomicStoreCoupledToWorker(instruction, cancel)
}

func callTakesCancellationOwnership(pass *analysis.Pass, instruction ssa.Instruction, cancel ssa.Value) bool {
	return ssautil.CallReturnsDeferredCleanup(instruction, cancel) ||
		lifecyclefacts.OwnsArgument(
			pass,
			"cancellationownership",
			string(check.CancellationRelease),
			instruction,
			cancel,
			func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
				return fact.Invoked | fact.ReturnedOwner
			},
		) ||
		lifecyclefacts.StoresInEscapingReceiver(
			pass,
			"cancellationownership",
			string(check.CancellationRelease),
			instruction,
			cancel,
		) ||
		ssautil.CallInvokesArgumentOnEveryReturn(instruction, cancel) ||
		ssautil.CallTransfersArgumentToReturnedOwner(instruction, cancel) ||
		ssautil.CallTransfersArgumentToReceiver(instruction, cancel) ||
		ssautil.CallTransfersArgumentToLifecycleOwner(instruction, cancel)
}

func atomicStoreCoupledToWorker(instruction ssa.Instruction, cancel ssa.Value) bool {
	// Atomic storage alone is not ownership: require the same external owner to
	// launch the worker whose lifecycle the slot controls.
	// https://github.com/jordigilh/kubernaut/blob/528b4f7080bf3522c0fa60f1ce87e48dcbcfe4bb/internal/kubernautagent/workflowcatalog/lazy_catalog.go#L100-L103
	common := ssautil.InstructionCall(instruction)
	if common == nil || ssautil.CallName(common) != "Store" || len(common.Args) < 2 ||
		!ssautil.ValueDerivesFrom(common.Args[len(common.Args)-1], cancel, map[ssa.Value]bool{}) &&
			!ssautil.ValueContainsValue(common.Args[len(common.Args)-1], cancel) {
		return false
	}
	field, ok := ssautil.CallReceiver(common).(*ssa.FieldAddr)
	if !ok || !ssautil.ExternallyOwnedValue(field.X) {
		return false
	}
	for _, block := range instruction.Parent().Blocks {
		for _, candidate := range block.Instrs {
			spawn, spawnOK := candidate.(*ssa.Go)
			if !spawnOK || !ssautil.InstructionMayFollow(instruction, spawn) {
				continue
			}
			if ssautil.ValueContainsValue(spawn.Common().Value, field.X) {
				return true
			}
			for _, argument := range spawn.Common().Args {
				if ssautil.SameValue(argument, field.X) {
					return true
				}
			}
		}
	}
	return false
}
