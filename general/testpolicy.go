package general

import (
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/checkutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func testPolicyAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "testpolicy",
		Doc:      "checks lifecycle ownership in test helpers",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runTestPolicy,
	}
}

func runTestPolicy(pass *analysis.Pass) (any, error) {
	for _, function := range checkutil.SourceSSAFunctions(pass) {
		file := checkutil.FunctionFile(pass, function)
		if file == nil || !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") || testEntryPoint(function.Name()) {
			continue
		}
		handle := testingSSAParameter(function)
		if handle == nil {
			continue
		}
		if checkutil.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
			common := checkutil.InstructionCall(instruction)
			return checkutil.CallName(common) == "Helper" && checkutil.AliasesValue(checkutil.CallReceiver(common), handle)
		}) {
			pass.Reportf(function.Pos(), "test helper accepting %s must call %s.Helper() on every return path", handle.Name(), handle.Name())
		}
	}
	return nil, nil
}

func testEntryPoint(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Fuzz")
}

func testingSSAParameter(function *ssa.Function) *ssa.Parameter {
	for _, parameter := range function.Params {
		if testingHandle(parameter.Type()) {
			return parameter
		}
	}
	return nil
}

func testingHandle(value types.Type) bool {
	pointer, ok := value.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := pointer.Elem().(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && (named.Obj().Name() == "T" || named.Obj().Name() == "B")
}
