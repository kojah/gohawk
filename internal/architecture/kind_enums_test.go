package architecture

import (
	"go/ast"
	"slices"
	"strings"
	"testing"
)

// Kind is an internal discriminator. Text at an output boundary should name
// its role (a noun, label, or prefix), rather than masquerading as an enum.
// This syntax gate covers raw fields, parameters, results and literal kind
// assignments, including tests. Named string domains are a separate review.
func TestNoRawKindEnums(t *testing.T) {
	t.Parallel()
	for _, source := range newRepositorySourceInventory(t).authoredGoFiles(t, ".") {
		ast.Inspect(source.file, func(node ast.Node) bool {
			if rawKindEnum(node) {
				t.Errorf("%s:%d: kind discriminators must use domain-owned numeric enums", source.repositoryPath, source.fileSet.Position(node.Pos()).Line)
			}
			return true
		})
	}
}

func rawKindEnum(node ast.Node) bool {
	isKind := func(name *ast.Ident) bool { return strings.EqualFold(name.Name, "kind") }
	switch node := node.(type) {
	case *ast.Field:
		underlying, ok := node.Type.(*ast.Ident)
		if !ok || underlying.Name != "string" {
			return false
		}
		return slices.ContainsFunc(node.Names, isKind)
	case *ast.ValueSpec:
		underlying, explicit := node.Type.(*ast.Ident)
		for index, name := range node.Names {
			if !isKind(name) {
				continue
			}
			if explicit && underlying.Name == "string" {
				return true
			}
			if len(node.Values) == len(node.Names) && reasonStringLiteral(node.Values[index]) {
				return true
			}
		}
	case *ast.AssignStmt:
		if len(node.Lhs) != len(node.Rhs) {
			return false
		}
		for index, left := range node.Lhs {
			name, ok := left.(*ast.Ident)
			if ok && isKind(name) && reasonStringLiteral(node.Rhs[index]) {
				return true
			}
		}
	}
	return false
}

func TestRawKindEnumMatcher(t *testing.T) {
	for _, test := range []struct {
		source string
		want   bool
	}{
		{`func f(kind string){}`, true},
		{`func f()(kind,title string){return "",""}`, true},
		{`type item struct{Kind string}`, true},
		{`var kind string`, true},
		{`func f(){kind:="value";_=kind}`, true},
		{`type queryKind uint8;func f(kind queryKind){}`, false},
		{`type words struct{noun string}`, false},
		{`func f(prefix string){}`, false},
		{`func f(){kind:=valueKind;_=kind}`, false},
		{`const fixture = "func f(kind string){}"`, false},
	} {
		assertReasonMatcher(t, test.source, test.want, rawKindEnum)
	}
}
