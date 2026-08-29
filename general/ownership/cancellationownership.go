package ownership

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	"github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func cancellationOwnershipAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "cancellationownership",
		Doc:      "checks context and signal-derived cancellation functions are called on every return path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
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
				cancel := resourceResult(call, contract.result)
				if cancel == nil {
					continue
				}
				// Cleanup need not occur in this function: returning the cancel,
				// storing it in an owner, or installing a callback that invokes it
				// transfers the obligation. Reassigned captured locals are included;
				// Prometheus installs its current cancel in a scraper callback:
				// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/scrape/scrape_test.go#L1294-L1315
				if ssautil.UnownedReturn(call, func(candidate ssa.Instruction) bool {
					return callsCancel(candidate, cancel)
				}, func(returned *ssa.Return) bool {
					return returnedValueAliases(returned, cancel)
				}) {
					source := analysisutil.SourceRange(pass, call.Pos())
					report(pass, checkCancellationRelease, analysis.Diagnostic{
						Pos:            source.Pos(),
						End:            source.End(),
						Message:        "cancel function from " + shortPackage(contract.packagePath) + "." + contract.name + " is not called on every return path",
						SuggestedFixes: cancellationFix(pass, call.Pos(), contract.name),
					})
				}
			}
		}
	}
	return nil, nil
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
				fix = []analysis.SuggestedFix{{
					Message: "Defer " + cancel.Name + " immediately after creation",
					TextEdits: []analysis.TextEdit{{
						Pos:     assignment.End(),
						NewText: []byte("\n\tdefer " + cancelInvocation(cancel.Name, constructor)),
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

func cancelInvocation(name, constructor string) string {
	if constructor == "WithCancelCause" {
		return name + "(nil)"
	}
	return name + "()"
}

func callsCancel(instruction ssa.Instruction, cancel ssa.Value) bool {
	if ssautil.ClosureCallsValue(instruction, cancel) || ssautil.ClosureOwnsValue(instruction, cancel) || ssautil.StoresValueInField(instruction, cancel) || ssautil.StoresValueInGlobal(instruction, cancel) || ssautil.StoresOwnerOfValueInField(instruction, cancel) || ssautil.StoresValueInOwnedMap(instruction, cancel) || ssautil.CallReturnsDeferredCleanup(instruction, cancel) {
		return true
	}
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if ssautil.AliasesValue(common.Value, cancel) {
		return true
	}
	name := strings.ToLower(ssautil.CallName(common))
	// Cleanup registrars own callbacks they receive, while Add/Register-style
	// APIs commonly store a cancellation function for a longer-lived owner.
	// Kubernetes uses both ginkgo.DeferCleanup and AddPodInPreBind this way.
	registersCallback := strings.Contains(name, "cleanup") || strings.Contains(name, "afterfunc")
	registersCancel := strings.HasPrefix(name, "add") || strings.Contains(name, "register") || strings.Contains(name, "track") || strings.Contains(name, "own")
	for _, argument := range common.Args {
		if registersCallback && ssautil.ValueCallsValue(argument, cancel) || registersCancel && ssautil.AliasesValue(argument, cancel) {
			return true
		}
	}
	if !strings.Contains(name, "cancel") && !strings.Contains(name, "stop") && name != "cleanup" {
		return false
	}
	for _, argument := range common.Args {
		if ssautil.AliasesValue(argument, cancel) {
			return true
		}
	}
	return false
}
