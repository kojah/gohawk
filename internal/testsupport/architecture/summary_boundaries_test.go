package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestSummaryInfrastructureBoundaries checks API use, not which evidence model an
// analyzer chooses. Completion proofs and context-keyed summaries remain valid
// alternatives to FunctionSummaries. Layering and traversal have their own
// tests; unknown propagation and cache correctness need behavioral tests.
func TestSummaryInfrastructureBoundaries(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal") {
		production[source.absolutePath] = source.repositoryPath
	}
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  inventory.root,
	}, "./internal/...")
	if err != nil {
		t.Fatal(err)
	}
	if count := packages.PrintErrors(loaded); count != 0 {
		t.Fatalf("load summary consumers: %d errors", count)
	}
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			path, ok := production[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				reason := summaryBoundaryViolation(pkg.TypesInfo, node)
				if !summaryRuleApplies(path, reason) {
					return true
				}
				t.Errorf("%s:%d: %s", path, pkg.Fset.Position(node.Pos()).Line, reason)
				return true
			})
		}
	}
}

func summaryRuleApplies(path, reason string) bool {
	if reason == "" || path == "internal/engine/ssaflow/calls/call_graph_memo.go" || path == "internal/engine/ssaflow/calls/call_summaries.go" {
		return false
	}
	// Shared searches retain the optional-budget contract of existing callers.
	// The mandatory-budget rule is for analyzer query sites; it does not change
	// fact inference semantics by silently imposing a new shared cutoff.
	return reason != summaryNilBudget || strings.HasPrefix(path, "internal/analysis/analyzers/")
}

const summaryNilBudget = "summary query passes nil budget; share a bounded SearchBudget with nested computation and binding"

func summaryBoundaryViolation(info *types.Info, node ast.Node) string {
	switch node := node.(type) {
	case *ast.CompositeLit:
		if summaryInfrastructureType(info.TypeOf(node)) {
			return "consumer constructs summary infrastructure state directly; use the shared constructors"
		}
	case *ast.SelectorExpr:
		selection := info.Selections[node]
		if !summaryInfrastructureSelection(selection) {
			return ""
		}
		if selection.Kind() == types.FieldVal {
			return "summary consumer accesses infrastructure state; use the summary query API, not cache or guard fields"
		}
		switch selection.Obj().Name() {
		case "Answer", "Enter", "Entered", "Leave", "Cut":
			return "consumer manages summary caching or recursion directly; use Summarize or Compose with WithFunction"
		}
	case *ast.CallExpr:
		selector, ok := ast.Unparen(node.Fun).(*ast.SelectorExpr)
		if !ok {
			return ""
		}
		selection := info.Selections[selector]
		if !summaryInfrastructureSelection(selection) {
			return ""
		}
		index := summaryBudgetIndex(selection)
		if index >= 0 && index < len(node.Args) && info.Types[ast.Unparen(node.Args[index])].IsNil() {
			return summaryNilBudget
		}
	}
	return ""
}

func summaryInfrastructureType(candidate types.Type) bool {
	if candidate == nil {
		return false
	}
	candidate = types.Unalias(candidate)
	if pointer, ok := candidate.(*types.Pointer); ok {
		candidate = types.Unalias(pointer.Elem())
	}
	named, ok := candidate.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != internalImportPrefix+"engine/ssaflow/calls" {
		return false
	}
	return named.Obj().Name() == "FunctionSummaries" || named.Obj().Name() == "CallGraphMemo"
}

func summaryInfrastructureSelection(selection *types.Selection) bool {
	if selection == nil {
		return false
	}
	if method, ok := selection.Obj().(*types.Func); ok {
		signature, ok := method.Type().(*types.Signature)
		return ok && signature.Recv() != nil && summaryInfrastructureType(signature.Recv().Type())
	}
	return summaryInfrastructureType(selection.Recv())
}

func summaryBudgetIndex(selection *types.Selection) int {
	index := -1
	switch selection.Obj().Name() {
	case "Function", "AtCall", "Compose":
		index = 1
	case "Summarize":
		index = 2
	}
	if index >= 0 && selection.Kind() == types.MethodExpr {
		index++ // An explicit receiver is the first argument of a method expression.
	}
	return index
}

// Synthetic packages keep the matcher tests independent of actual API shape
// and let us test forbidden public fields without exposing real cache state.
const summaryAPIFixture = `package calls
type SearchBudget struct{}
type FunctionSummaries[T any] struct { Cache map[int]T }
func (*FunctionSummaries[T]) Function(int, *SearchBudget) {}
func (*FunctionSummaries[T]) AtCall(int, *SearchBudget, func()) {}
type CallGraphMemo[K comparable, V any] struct{}
func (*CallGraphMemo[K,V]) Summarize(K, int, *SearchBudget, func(), func()) {}
func (*CallGraphMemo[K,V]) Compose(K, *SearchBudget, func(), func()) {}
func (*CallGraphMemo[K,V]) WithFunction(int, func()) {}
func (*CallGraphMemo[K,V]) Incomplete() {}
func (*CallGraphMemo[K,V]) Answer() {}
func (*CallGraphMemo[K,V]) Enter() {}
func (*CallGraphMemo[K,V]) Entered() {}
func (*CallGraphMemo[K,V]) Leave() {}
func (*CallGraphMemo[K,V]) Cut() {}
`

func TestSummaryBoundaryMatcher(t *testing.T) {
	api, _, _ := checkSummarySource(t, internalImportPrefix+"engine/ssaflow/calls", summaryAPIFixture, nil)
	for _, test := range []struct {
		name, body, reason string
	}{
		{"bounded function", "s.Function(0, budget)", ""},
		{"bounded call", "s.AtCall(0, budget, nil)", ""},
		{"bounded context", "m.Summarize(0, 0, budget, compute, unavailable)", ""},
		{"bounded composition", "m.Compose(0, budget, compute, unavailable)", ""},
		{"guarded visit", "m.WithFunction(0, compute)", ""},
		{"partial evidence", "m.Incomplete()", ""},
		{"nil function budget", "s.Function(0, nil)", summaryNilBudget},
		{"nil call budget", "s.AtCall(0, (nil), nil)", summaryNilBudget},
		{"nil context budget", "m.Summarize(0, 0, nil, compute, unavailable)", summaryNilBudget},
		{"nil composition budget", "m.Compose(0, nil, compute, unavailable)", summaryNilBudget},
		{"method expression", "(*flow.FunctionSummaries[int]).Function(s, 0, nil)", summaryNilBudget},
		{"context expression", "(*flow.CallGraphMemo[int,int]).Summarize(m, 0, 0, nil, compute, unavailable)", summaryNilBudget},
		{"cache access", "_ = s.Cache", "accesses infrastructure state"},
		{"direct cache construction", "_ = flow.CallGraphMemo[int,int]{}", "constructs summary infrastructure state"},
		{"direct summary construction", "_ = flow.FunctionSummaries[int]{}", "constructs summary infrastructure state"},
		{"raw answer", "m.Answer()", "manages summary caching"},
		{"raw guard", "m.Enter()", "manages summary caching"},
		{"guard method value", "_ = m.Entered", "manages summary caching"},
		{"raw leave", "m.Leave()", "manages summary caching"},
		{"raw cut", "m.Cut()", "manages summary caching"},
		{"type alias", "var alias *Alias; alias.Enter()", "manages summary caching"},
		{"embedded method", "var wrap Wrapper; wrap.Enter()", "manages summary caching"},
		{"unrelated names", "var other unrelated; other.Enter(); other.Function(0, nil)", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `package consumer
import flow "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
type Alias = flow.CallGraphMemo[int,int]
type Wrapper struct { *flow.CallGraphMemo[int,int] }
type unrelated struct{}
func (unrelated) Enter() {}
func (unrelated) Function(int, *flow.SearchBudget) {}
func use(s *flow.FunctionSummaries[int], m *flow.CallGraphMemo[int,int], budget *flow.SearchBudget, compute, unavailable func()) {
` + test.body + "\n}"
			_, info, file := checkSummarySource(t, "consumer", source, summaryFixtureImporter{api})
			var reasons []string
			ast.Inspect(file, func(node ast.Node) bool {
				if reason := summaryBoundaryViolation(info, node); reason != "" {
					reasons = append(reasons, reason)
				}
				return true
			})
			if test.reason == "" && len(reasons) != 0 {
				t.Errorf("accepted form rejected: %v", reasons)
			} else if test.reason != "" && (len(reasons) != 1 || !strings.Contains(reasons[0], test.reason)) {
				t.Errorf("reasons = %v, want one containing %q", reasons, test.reason)
			}
		})
	}
	t.Run("scope", testSummaryRuleScopes)
}

func testSummaryRuleScopes(t *testing.T) {
	for _, path := range []string{
		"internal/analysis/analyzers/example/proof.go", "internal/engine/lifecycle/completion_search.go", "internal/analysis/passes/lifecyclefacts/fields.go",
	} {
		if !summaryRuleApplies(path, "raw guard operation") || !summaryRuleApplies(path, "cache field access") {
			t.Errorf("%s escaped shared infrastructure enforcement", path)
		}
	}
	for _, path := range []string{"internal/engine/ssaflow/calls/call_graph_memo.go", "internal/engine/ssaflow/calls/call_summaries.go"} {
		if summaryRuleApplies(path, "raw guard operation") {
			t.Errorf("implementation owner %s rejected", path)
		}
	}
	if !summaryRuleApplies("internal/analysis/analyzers/example/proof.go", summaryNilBudget) ||
		summaryRuleApplies("internal/engine/lifecycle/completion_search.go", summaryNilBudget) {
		t.Error("analyzer budget requirement must not silently redefine optional-budget shared APIs")
	}
}

type summaryFixtureImporter struct{ pkg *types.Package }

func (loader summaryFixtureImporter) Import(path string) (*types.Package, error) {
	if path == loader.pkg.Path() {
		return loader.pkg, nil
	}
	return nil, fmt.Errorf("unexpected fixture import %q", path)
}

func checkSummarySource(t *testing.T, path, source string, importer types.Importer) (*types.Package, *types.Info, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue), Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	config := types.Config{Importer: importer}
	pkg, err := config.Check(path, fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	return pkg, info, file
}

// Summary components may be typed in analyzer code, but their prerequisites,
// construction, and fact-backed lookup enter through the summary broker.
// Checking resolved objects catches import aliases, dot imports, and assigning
// constructors to function variables rather than only direct selector calls.
func TestAnalyzersUseSummaryBroker(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal/analysis/analyzers") {
		production[source.absolutePath] = source.repositoryPath
	}
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  inventory.root,
	}, "./internal/analysis/analyzers/...")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load summary consumers: %d errors", errors)
	}
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			path, ok := production[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if summaryBrokerNodeForbidden(pkg.TypesInfo, node) {
					t.Errorf("%s:%d bypasses the summary broker; obtain selected knowledge through internal/analysis/summaries",
						path, pkg.Fset.Position(node.Pos()).Line)
				}
				return true
			})
		}
	}
}

func summaryBrokerNodeForbidden(info *types.Info, node ast.Node) bool {
	switch node := node.(type) {
	case *ast.Ident:
		return summaryBrokerObjectForbidden(info.Uses[node])
	case *ast.CompositeLit:
		return summaryBrokerConstructionForbidden(info.TypeOf(node))
	case *ast.TypeAssertExpr:
		return summaryBrokerConstructionForbidden(info.TypeOf(node.Type))
	case *ast.CallExpr:
		identifier, ok := node.Fun.(*ast.Ident)
		if !ok {
			return false
		}
		builtin, ok := info.Uses[identifier].(*types.Builtin)
		return ok && builtin.Name() == "new" && summaryBrokerConstructionForbidden(info.TypeOf(node))
	default:
		return false
	}
}

func summaryDomainPackage(path string) bool {
	return path == internalImportPrefix+"analysis/passes/lifecyclefacts" || path == internalImportPrefix+"analysis/passes/concurrencyfacts" ||
		path == internalImportPrefix+"analysis/passes/resultfacts"
}

func summaryBrokerObjectForbidden(object types.Object) bool {
	if object == nil || object.Pkg() == nil || !summaryDomainPackage(object.Pkg().Path()) {
		return false
	}
	if variable, ok := object.(*types.Var); ok {
		return !variable.IsField() && variable.Name() == "Analyzer"
	}
	function, ok := object.(*types.Func)
	if !ok {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() != nil {
		return false
	}
	// These helpers inspect type/value structure without obtaining summaries
	// or reading pass-owned knowledge. New exceptions require explicit review.
	path := function.Pkg().Path()
	if path == internalImportPrefix+"analysis/passes/lifecyclefacts" && (function.Name() == "ResourceCleanup" || function.Name() == "ResponseBodyField") {
		return false
	}
	if path == internalImportPrefix+"analysis/passes/concurrencyfacts" && function.Name() == "MutexPointer" {
		return false
	}
	return true
}

func summaryBrokerConstructionForbidden(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || !summaryDomainPackage(named.Obj().Pkg().Path()) {
		return false
	}
	switch named.Obj().Name() {
	case "Engine", "LifecycleEvidence", "Summaries", "Fact":
		return true
	default:
		return false
	}
}

type summaryBrokerImporter struct{ pkg *types.Package }

func (importer summaryBrokerImporter) Import(string) (*types.Package, error) {
	return importer.pkg, nil
}

func TestSummaryBrokerMatchesDeclarationIdentity(t *testing.T) {
	for _, test := range []struct {
		name, path, source string
		want               int
	}{
		{"alias", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import f "domain"; var _ = f.NewLifecycleEvidence`, 1},
		{"dot", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import . "domain"; var _ = NewLifecycleEvidence`, 1},
		{"prerequisite", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import f "domain"; var _ = f.Analyzer`, 1},
		{"structure", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import f "domain"; var _ = f.ResourceCleanup`, 0},
		{"lookalike", "example.org/lifecyclefacts", `package p; import f "domain"; var _ = f.NewLifecycleEvidence`, 0},
		{"literal", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import f "domain"; var _ = f.Engine{}`, 1},
		{"new", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import f "domain"; var _ = new(f.Engine)`, 1},
		{"type-alias", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import f "domain"; type E = f.Engine; var _ = E{}`, 1},
		{"result-assertion", internalImportPrefix + "analysis/passes/lifecyclefacts", `package p; import f "domain"; var v any; var _ = v.(*f.Engine)`, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			domain := types.NewPackage(test.path, "domain")
			for _, name := range []string{"NewLifecycleEvidence", "ResourceCleanup"} {
				domain.Scope().Insert(types.NewFunc(token.NoPos, domain, name, types.NewSignatureType(nil, nil, nil, nil, nil, false)))
			}
			domain.Scope().Insert(types.NewVar(token.NoPos, domain, "Analyzer", types.Typ[types.Int]))
			engine := types.NewTypeName(token.NoPos, domain, "Engine", nil)
			types.NewNamed(engine, types.NewStruct(nil, nil), nil)
			domain.Scope().Insert(engine)
			domain.MarkComplete()
			files := token.NewFileSet()
			file, err := parser.ParseFile(files, "consumer.go", test.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Uses: make(map[*ast.Ident]types.Object), Types: make(map[ast.Expr]types.TypeAndValue)}
			config := types.Config{Importer: summaryBrokerImporter{pkg: domain}}
			if _, err := config.Check("consumer", files, []*ast.File{file}, info); err != nil {
				t.Fatal(err)
			}
			found := 0
			ast.Inspect(file, func(node ast.Node) bool {
				if summaryBrokerNodeForbidden(info, node) {
					found++
				}
				return true
			})
			if found != test.want {
				t.Fatalf("got %d bypasses, want %d", found, test.want)
			}
		})
	}
}
