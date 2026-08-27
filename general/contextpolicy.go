package general

import (
	"go/ast"
	"go/types"
	"go/version"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

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
	analyzer.Flags.BoolVar(&config.preferTestContext, "prefer-test-context", true, "prefer t.Context or b.Context over context.Background in tests")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runContextPolicy(pass, config)
	}
	return analyzer
}

type contextPolicyConfig struct {
	preferTestContext bool
}

func runContextPolicy(pass *analysis.Pass, config contextPolicyConfig) (any, error) {
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		isTest := strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
		ast.Inspect(file, func(node ast.Node) bool {
			checkContextStructure(pass, node, isTest && supportsTestingContext(pass), config)
			return true
		})
	}
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
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

func checkContextStructure(pass *analysis.Pass, node ast.Node, isTest bool, config contextPolicyConfig) {
	switch typed := node.(type) {
	case *ast.FuncDecl:
		for index, parameter := range parameterTypes(pass, typed.Type.Params) {
			if isContext(parameter) && index != 0 {
				pass.Reportf(typed.Name.Pos(), "context.Context must be first parameter")
				break
			}
		}
	case *ast.StructType:
		for _, field := range typed.Fields.List {
			if isContext(pass.TypesInfo.TypeOf(field.Type)) {
				pass.Reportf(field.Pos(), "do not store context.Context in a struct")
			}
		}
	case *ast.CallExpr:
		if config.preferTestContext && isTest && analysisutil.IsPackageCall(pass, typed, analysisutil.FunctionSymbol{Package: "context", Name: "Background"}) {
			pass.Reportf(typed.Pos(), "use t.Context() or b.Context() instead of context.Background()")
		}
	}
}

func isContext(value types.Type) bool {
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "context" && named.Obj().Name() == "Context"
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
		if isContext(signature.Params().At(index).Type()) && definitelyNil(common.Args[argumentIndex], map[ssa.Value]bool{}) {
			pass.Reportf(call.Pos(), "do not pass nil context.Context")
		}
	}
}

func definitelyNil(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if literal, ok := value.(*ssa.Const); ok {
		return literal.IsNil()
	}
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return definitelyNil(typed.X, seen)
	case *ssa.ChangeType:
		return definitelyNil(typed.X, seen)
	case *ssa.Convert:
		return definitelyNil(typed.X, seen)
	case *ssa.MakeInterface:
		return definitelyNil(typed.X, seen)
	case *ssa.Phi:
		if len(typed.Edges) == 0 {
			return false
		}
		for _, edge := range typed.Edges {
			if !definitelyNil(edge, seen) {
				return false
			}
		}
		return true
	}
	return false
}
