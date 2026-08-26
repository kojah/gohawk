package general

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

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
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
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
				if analysisutil.UnownedReturn(call, func(candidate ssa.Instruction) bool {
					return callsCancel(candidate, cancel)
				}, func(returned *ssa.Return) bool {
					return returnedValueAliases(returned, cancel)
				}) {
					pass.Report(analysis.Diagnostic{
						Pos:            call.Pos(),
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
	packagePath, name := analysisutil.CallPackage(common), analysisutil.CallName(common)
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
	if analysisutil.ClosureCallsValue(instruction, cancel) || analysisutil.ClosureOwnsValue(instruction, cancel) || analysisutil.StoresValueInField(instruction, cancel) || analysisutil.StoresValueInOwnedMap(instruction, cancel) {
		return true
	}
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if analysisutil.AliasesValue(common.Value, cancel) {
		return true
	}
	name := strings.ToLower(analysisutil.CallName(common))
	if !strings.Contains(name, "cancel") && !strings.Contains(name, "stop") && name != "cleanup" {
		return false
	}
	for _, argument := range common.Args {
		if analysisutil.AliasesValue(argument, cancel) {
			return true
		}
	}
	return false
}
