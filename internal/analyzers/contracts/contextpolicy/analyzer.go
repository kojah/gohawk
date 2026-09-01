// Package contextpolicy implements the contextpolicy gohawk analyzer.
package contextpolicy

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "contextpolicy",
		Doc:      "checks context placement, storage, and nil use",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runContextPolicy,
	}
}

func runContextPolicy(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, file := range pass.Files {
		if !syntax.AnalyzeFile(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			checkContextStructure(pass, file, node)
			return true
		})
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if ok {
					reportNilSSAContextArguments(pass, call)
				}
			}
		}
	}
	return nil, nil
}

func checkContextStructure(pass *analysis.Pass, file *ast.File, node ast.Node) {
	switch typed := node.(type) {
	case *ast.FuncDecl:
		parameters := syntax.ParameterTypes(pass, typed.Type.Params)
		for index, parameter := range parameters {
			if syntax.NamedType(parameter, "context", "Context") && !validContextPosition(parameters, index) {
				check.Reportf(pass, check.ContextFirst, typed.Name.Pos(), "context.Context must be first parameter")
				break
			}
		}
	case *ast.TypeSpec:
		structure, ok := typed.Type.(*ast.StructType)
		if !ok || dedicatedContextCarrier(typed.Name.Name) || testOwnedContextCarrier(pass, file, typed.Name.Name) || ownsStoredContext(pass, structure) {
			return
		}
		for _, field := range structure.Fields.List {
			if syntax.NamedType(pass.TypesInfo.TypeOf(field.Type), "context", "Context") {
				check.Reportf(pass, check.ContextStorage, field.Pos(), "do not store context.Context in a struct")
			}
		}
	}
}

func dedicatedContextCarrier(name string) bool {
	// A type explicitly named as a context or operation transition makes the
	// bounded carrier role visible instead of hiding a request context in
	// arbitrary state. Garage Operator passes a one-reconcile snapshot between
	// lifecycle phases using this contract:
	// https://github.com/rajsinghtech/garage-operator/blob/b2b2b1776e0f344f68f901eae27c2d52b04dfd4e/internal/controller/node_local_pool_lifecycle_phases.go#L55-L83
	return strings.HasSuffix(name, "Context") || strings.HasSuffix(name, "Transition")
}

func testOwnedContextCarrier(pass *analysis.Pass, file *ast.File, name string) bool {
	if !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
		return false
	}
	// Private test structs and explicitly named TestCase/Fixture carriers have a
	// lifetime owned by the test binary. Other exported test types remain visible
	// so examples of production contracts are still checked.
	return !ast.IsExported(name) || strings.HasSuffix(name, "TestCase") || strings.HasSuffix(name, "Fixture")
}

func validContextPosition(parameters []types.Type, index int) bool {
	if index == 0 {
		return true
	}
	// Once a context leads the API, a second context can represent a distinct
	// lifecycle rather than misplaced plumbing. Network Doctor uses parent and
	// server contexts this way:
	// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/peer/session.go#L858
	if syntax.NamedType(parameters[0], "context", "Context") {
		return true
	}
	// Testing handles conventionally lead helper signatures because they own
	// failures and cleanup. Treat the following context as the first application
	// parameter instead of asking helpers to put ctx before t or b.
	return index == 1 && (syntax.NamedType(parameters[0], "testing", "T") || syntax.NamedType(parameters[0], "testing", "B"))
}

func ownsStoredContext(pass *analysis.Pass, structure *ast.StructType) bool {
	hasContext := false
	hasLifecycleHandle := false
	for _, field := range structure.Fields.List {
		fieldType := pass.TypesInfo.TypeOf(field.Type)
		if syntax.NamedType(fieldType, "context", "Context") {
			hasContext = true
			for _, name := range field.Names {
				lower := strings.ToLower(name.Name)
				if strings.Contains(lower, "parent") || strings.Contains(lower, "base") || strings.Contains(lower, "shutdown") ||
					strings.Contains(lower, "lifecycle") {
					hasLifecycleHandle = true
				}
			}
		}
		hasLifecycleHandle = hasLifecycleHandle || syntax.NamedType(fieldType, "context", "CancelFunc") ||
			syntax.NamedType(fieldType, "sync", "WaitGroup")
	}
	// A cancel/join handle or an explicitly lifecycle-named context is strong
	// evidence that the struct owns a bounded component rather than retaining a
	// request context as data. This pattern is paired with Close/Wait machinery
	// in Network Doctor's servers:
	// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/simulation/httpconnect.go#L40-L57
	return hasContext && hasLifecycleHandle
}

func reportNilSSAContextArguments(pass *analysis.Pass, call *ssa.Call) {
	common := call.Common()
	signature := common.Signature()
	if signature == nil {
		return
	}
	offset := 0
	if signature.Recv() != nil && !common.IsInvoke() {
		offset = 1
	}
	for index := range signature.Params().Len() {
		argumentIndex := index + offset
		if argumentIndex >= len(common.Args) {
			break
		}
		if syntax.NamedType(signature.Params().At(index).Type(), "context", "Context") && ssaflow.DefinitelyNil(common.Args[argumentIndex]) {
			check.Reportf(pass, check.ContextNilArgument, call.Pos(), "do not pass nil context.Context")
		}
	}
}
