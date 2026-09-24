package architecture

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// The baseline is migration debt, not permission to add new string reasons.
// Keep reducing it as complete domains migrate. Trace DTOs and observers are
// the serialization boundary; their textual schema is deliberately unchanged.
func TestReasonMigrationDebt(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	data, err := os.ReadFile(filepath.Join(inventory.root, "internal/architecture/testdata/reason-string-baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var baseline map[string]int
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for _, source := range inventory.productionGoFiles(t, "internal") {
		if filepath.Dir(source.repositoryPath) == "internal/trace" || source.repositoryPath == "internal/ssaflow/proof_observer.go" {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			if rawReasonClassification(node) {
				counts[source.repositoryPath]++
			}
			return true
		})
	}
	for path, count := range counts {
		if count != baseline[path] {
			t.Errorf("reason migration debt %s: got %d, baseline %d; migrate reasons or reduce a stale baseline", path, count, baseline[path])
		}
		delete(baseline, path)
	}
	for path, count := range baseline {
		t.Errorf("reason migration debt %s: got 0, baseline %d; remove the completed entry", path, count)
	}
}

func rawReasonClassification(node ast.Node) bool {
	switch node := node.(type) {
	case *ast.TypeSpec:
		return reasonEnumViolation(node)
	case *ast.Field:
		if kind, ok := node.Type.(*ast.Ident); ok && kind.Name == "string" {
			for _, name := range node.Names {
				if name.Name == "reason" || name.Name == "Reason" {
					return true
				}
			}
		}
	case *ast.KeyValueExpr:
		key, ok := node.Key.(*ast.Ident)
		return ok && (key.Name == "Reason" || key.Name == "reason") && reasonStringLiteral(node.Value)
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

func reasonAssignmentTarget(expression ast.Expr) bool {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name == "reason"
	case *ast.SelectorExpr:
		return expression.Sel.Name == "Reason" || expression.Sel.Name == "reason"
	}
	return false
}

func reasonStringLiteral(expression ast.Expr) bool {
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
		{"var proof = Proof{Reason: \"unknown\"}", true},
		{"type QueryReason uint8; type Proof struct { Reason QueryReason }", false},
		{"func f() { proof.Reason = UnknownReason }", false},
		{"func f() string { return \"display text\" }", false},
	} {
		assertReasonMatcher(t, test.source, test.want, rawReasonClassification)
	}
}
