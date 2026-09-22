package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// Synthetic packages keep the matcher tests independent of actual API shape
// and let us test forbidden public fields without exposing real cache state.
const summaryAPIFixture = `package ssaflow
type SearchBudget struct{}
type FunctionSummaries[T any] struct { Cache map[int]T }
func (*FunctionSummaries[T]) Function(int, *SearchBudget) {}
func (*FunctionSummaries[T]) AtCall(int, *SearchBudget, func()) {}
type CallGraphMemo[K comparable, V any] struct{}
func (*CallGraphMemo[K,V]) Summarize(K, int, *SearchBudget, func(), func()) {}
func (*CallGraphMemo[K,V]) Answer() {}
func (*CallGraphMemo[K,V]) Enter() {}
func (*CallGraphMemo[K,V]) Entered() {}
func (*CallGraphMemo[K,V]) Leave() {}
func (*CallGraphMemo[K,V]) Cut() {}
`

func TestSummaryBoundaryMatcher(t *testing.T) {
	api, _, _ := checkSummarySource(t, internalImportPrefix+"ssaflow", summaryAPIFixture, nil)
	for _, test := range []struct {
		name, body, reason string
	}{
		{"bounded function", "s.Function(0, budget)", ""},
		{"bounded call", "s.AtCall(0, budget, nil)", ""},
		{"bounded context", "m.Summarize(0, 0, budget, compute, unavailable)", ""},
		{"nil function budget", "s.Function(0, nil)", summaryNilBudget},
		{"nil call budget", "s.AtCall(0, (nil), nil)", summaryNilBudget},
		{"nil context budget", "m.Summarize(0, 0, nil, compute, unavailable)", summaryNilBudget},
		{"method expression", "(*flow.FunctionSummaries[int]).Function(s, 0, nil)", summaryNilBudget},
		{"context expression", "(*flow.CallGraphMemo[int,int]).Summarize(m, 0, 0, nil, compute, unavailable)", summaryNilBudget},
		{"cache access", "_ = s.Cache", "accesses infrastructure state"},
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
import flow "github.com/kojah/gohawk/internal/ssaflow"
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
