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
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
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
		parameters := parameterTypes(pass, typed.Type.Params)
		for index, parameter := range parameters {
			if analysisutil.NamedType(parameter, "context", "Context") && !validContextPosition(parameters, index) {
				reportf(pass, checkContextFirst, typed.Name.Pos(), "context.Context must be first parameter")
				break
			}
		}
	case *ast.StructType:
		if ownsStoredContext(pass, typed) {
			return
		}
		for _, field := range typed.Fields.List {
			if analysisutil.NamedType(pass.TypesInfo.TypeOf(field.Type), "context", "Context") {
				reportf(pass, checkContextStorage, field.Pos(), "do not store context.Context in a struct")
			}
		}
	case *ast.CallExpr:
		if config.preferTestContext && isTest && analysisutil.IsPackageCall(pass, typed, analysisutil.FunctionSymbol{Package: "context", Name: "Background"}) {
			reportf(pass, checkContextTestOwnership, typed.Pos(), "use t.Context() or b.Context() instead of context.Background()")
		}
	}
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
	hasCancel := false
	for _, field := range structure.Fields.List {
		fieldType := pass.TypesInfo.TypeOf(field.Type)
		hasContext = hasContext || analysisutil.NamedType(fieldType, "context", "Context")
		hasCancel = hasCancel || analysisutil.NamedType(fieldType, "context", "CancelFunc")
	}
	// A typed cancel handle is strong evidence that the struct owns a bounded
	// component lifecycle rather than retaining a request context as data. This
	// pattern is paired with Close/Wait machinery in Network Doctor's servers:
	// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/simulation/httpconnect.go#L40-L57
	return hasContext && hasCancel
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
