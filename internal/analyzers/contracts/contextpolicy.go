package contracts

import (
	"go/ast"
	"go/types"
	"go/version"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	"github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func contextPolicyAnalyzer() *analysis.Analyzer {
	config := contextPolicyConfig{preferTestContext: true}
	analyzer := &analysis.Analyzer{
		Name:     "contextpolicy",
		Doc:      "checks context placement, storage, nil use, and test ownership",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
	}
	analyzer.Flags.BoolVar(&config.preferTestContext, "prefer-test-context", true, "check detached test-owned goroutines rooted in a never-cancelled context")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runContextPolicy(pass, config)
	}
	return analyzer
}

type contextPolicyConfig struct {
	preferTestContext bool
}

func runContextPolicy(pass *analysis.Pass, config contextPolicyConfig) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
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
		if config.preferTestContext && supportsTestingContext(pass) {
			reportDetachedTestBackground(pass, function)
		}
	}
	return nil, nil
}

func supportsTestingContext(pass *analysis.Pass) bool {
	if pass.Module == nil || pass.Module.GoVersion == "" {
		return true
	}
	moduleVersion := pass.Module.GoVersion
	if !strings.HasPrefix(moduleVersion, "go") {
		moduleVersion = "go" + moduleVersion
	}
	return version.Compare(moduleVersion, "go1.24") >= 0
}

func checkContextStructure(pass *analysis.Pass, file *ast.File, node ast.Node) {
	switch typed := node.(type) {
	case *ast.FuncDecl:
		parameters := parameterTypes(pass, typed.Type.Params)
		for index, parameter := range parameters {
			if analysisutil.NamedType(parameter, "context", "Context") && !validContextPosition(parameters, index) {
				reportf(pass, checkContextFirst, typed.Name.Pos(), "context.Context must be first parameter")
				break
			}
		}
	case *ast.TypeSpec:
		structure, ok := typed.Type.(*ast.StructType)
		if !ok || dedicatedContextCarrier(typed.Name.Name) || testOwnedContextCarrier(pass, file, typed.Name.Name) || ownsStoredContext(pass, structure) {
			return
		}
		for _, field := range structure.Fields.List {
			if analysisutil.NamedType(pass.TypesInfo.TypeOf(field.Type), "context", "Context") {
				reportf(pass, checkContextStorage, field.Pos(), "do not store context.Context in a struct")
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
	if analysisutil.NamedType(parameters[0], "context", "Context") {
		return true
	}
	// Testing handles conventionally lead helper signatures because they own
	// failures and cleanup. Treat the following context as the first application
	// parameter instead of asking helpers to put ctx before t or b.
	return index == 1 && (analysisutil.NamedType(parameters[0], "testing", "T") || analysisutil.NamedType(parameters[0], "testing", "B"))
}

func ownsStoredContext(pass *analysis.Pass, structure *ast.StructType) bool {
	hasContext := false
	hasLifecycleHandle := false
	for _, field := range structure.Fields.List {
		fieldType := pass.TypesInfo.TypeOf(field.Type)
		if analysisutil.NamedType(fieldType, "context", "Context") {
			hasContext = true
			for _, name := range field.Names {
				lower := strings.ToLower(name.Name)
				if strings.Contains(lower, "parent") || strings.Contains(lower, "base") || strings.Contains(lower, "shutdown") || strings.Contains(lower, "lifecycle") {
					hasLifecycleHandle = true
				}
			}
		}
		hasLifecycleHandle = hasLifecycleHandle || analysisutil.NamedType(fieldType, "context", "CancelFunc") || analysisutil.NamedType(fieldType, "sync", "WaitGroup")
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
		if analysisutil.NamedType(signature.Params().At(index).Type(), "context", "Context") && ssautil.DefinitelyNil(common.Args[argumentIndex]) {
			reportf(pass, checkContextNilArgument, call.Pos(), "do not pass nil context.Context")
		}
	}
}
