package architecture

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// Raw reason classifications are forbidden outside the textual output boundary.
// Syntax checks cover common forms; review must also catch unnamed return
// values and synthesized codes whose semantic role is not evident from syntax.
func TestNoRawReasonClassifications(t *testing.T) {
	t.Parallel()
	for _, source := range newRepositorySourceInventory(t).productionGoFiles(t, ".") {
		if reasonTextBoundary(source.repositoryPath) {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			if reasonEnumViolation(node) || rawReasonClassification(node) {
				t.Errorf("%s:%d: reason classifications must use domain-owned enums", source.repositoryPath, source.fileSet.Position(node.Pos()).Line)
			}
			return true
		})
	}
}

func rawReasonClassification(node ast.Node) bool {
	switch node := node.(type) {
	case *ast.TypeSpec:
		return reasonEnumViolation(node)
	case *ast.Field:
		return rawReasonField(node)
	case *ast.KeyValueExpr:
		key, ok := node.Key.(*ast.Ident)
		return ok && (key.Name == "Reason" || key.Name == "reason") && reasonStringLiteral(node.Value)
	case *ast.ValueSpec:
		if len(node.Names) != len(node.Values) {
			return false
		}
		for index, name := range node.Names {
			if reasonAssignmentTarget(name) && reasonStringLiteral(node.Values[index]) {
				return true
			}
		}
	case *ast.AssignStmt:
		if len(node.Lhs) != len(node.Rhs) {
			return false
		}
		for index, left := range node.Lhs {
			if reasonAssignmentTarget(left) && reasonStringLiteral(node.Rhs[index]) {
				return true
			}
		}
	}
	return false
}

func rawReasonField(field *ast.Field) bool {
	kind, ok := field.Type.(*ast.Ident)
	if !ok || kind.Name != "string" {
		return false
	}
	for _, name := range field.Names {
		if name.Name == "reason" || strings.HasSuffix(name.Name, "Reason") {
			return true
		}
	}
	return false
}

func reasonAssignmentTarget(expression ast.Expr) bool {
	switch expression := expression.(type) {
	case *ast.Ident:
		return strings.HasPrefix(expression.Name, "reason") || strings.HasSuffix(expression.Name, "Reason")
	case *ast.SelectorExpr:
		return expression.Sel.Name == "Reason" || expression.Sel.Name == "reason"
	}
	return false
}

func reasonStringLiteral(expression ast.Expr) bool {
	if binary, ok := ast.Unparen(expression).(*ast.BinaryExpr); ok && binary.Op == token.ADD {
		return reasonStringLiteral(binary.X) || reasonStringLiteral(binary.Y)
	}
	literal, ok := ast.Unparen(expression).(*ast.BasicLit)
	return ok && literal.Kind == token.STRING
}

func TestRawReasonClassificationMatcher(t *testing.T) {
	for _, test := range []struct {
		source string
		want   bool
	}{
		{"type Proof struct { Reason string }", true},
		{"type Reason string", true},
		{"func f(reason string) {}", true},
		{"func f() { reason := \"unknown\" }", true},
		{"func f() { proof.Reason = \"unknown\" }", true},
		{"const reasonUnknown = \"unknown\"", true},
		{"func f() { reason := \"prefix-\" + suffix }", true},
		{"var missingReason = \"unknown\"", true},
		{"var proof = Proof{Reason: \"unknown\"}", true},
		{"type QueryReason uint8; type Proof struct { Reason QueryReason }", false},
		{"func f() { proof.Reason = UnknownReason }", false},
		{"func f() string { return \"display text\" }", false},
	} {
		assertReasonMatcher(t, test.source, test.want, rawReasonClassification)
	}
}
