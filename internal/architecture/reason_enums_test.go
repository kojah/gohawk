package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Grow this scope as reason domains migrate. Trace observers and wire payloads
// remain textual boundaries; internal reason types and stored fields do not.
func TestMigratedReasonEnums(t *testing.T) {
	t.Parallel()
	for _, source := range newRepositorySourceInventory(t).productionGoFiles(t,
		"internal/heapmodel", "internal/passes/resultfacts", "internal/analyzers/resources/cancellationownership",
		"internal/analyzers/concurrency/producerlifecycle", "internal/analyzers/resources/processownership",
		"internal/analyzers/correctness/nilargument", "internal/analyzers/concurrency/concurrentcapture",
		"internal/analyzers/resources/deferinloop", "internal/check", "internal/cli",
		"internal/passes/concurrencyfacts", "internal/passes/lifecyclefacts", "internal/syncmodel",
		"internal/analyzers/concurrency/lockorder", "internal/analyzers/concurrency/channelsafety",
		"internal/analyzers/concurrency/goroutineownership") {
		ast.Inspect(source.file, func(node ast.Node) bool {
			if reasonEnumViolation(node) {
				t.Errorf("%s:%d: internal reasons require domain-owned numeric enums",
					source.repositoryPath, source.fileSet.Position(node.Pos()).Line)
			}
			return true
		})
	}
}

func reasonEnumViolation(node ast.Node) bool {
	switch node := node.(type) {
	case *ast.TypeSpec:
		if !strings.HasSuffix(node.Name.Name, "Reason") {
			return false
		}
		underlying, ok := node.Type.(*ast.Ident)
		if !ok || node.Assign.IsValid() {
			return true
		}
		switch underlying.Name {
		case "uint8", "uint16", "uint32", "uint64", "uint", "int8", "int16", "int32", "int64", "int":
			return false
		}
		return true
	case *ast.StructType:
		for _, field := range node.Fields.List {
			underlying, ok := field.Type.(*ast.Ident)
			if !ok || underlying.Name != "string" {
				continue
			}
			for _, name := range field.Names {
				if name.Name == "reason" || strings.HasSuffix(name.Name, "Reason") {
					return true
				}
			}
		}
	}
	return false
}

func TestReasonEnumBoundaryMatcher(t *testing.T) {
	for _, test := range []struct {
		source string
		want   bool
	}{
		{"type QueryReason uint8", false},
		{"type QueryReason string", true},
		{"type QueryReason = uint8", true},
		{"type Proof struct { Reason string }", true},
		{"type Proof struct { reason string }", true},
		{"type Proof struct { Reason QueryReason }", false},
		{"type Observer func(reason string)", false},
		{"func (reason QueryReason) String() string { return \"unknown\" }", false},
	} {
		assertReasonMatcher(t, test.source, test.want, reasonEnumViolation)
	}
}

func assertReasonMatcher(t *testing.T, source string, want bool, match func(ast.Node) bool) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "reason.go", "package fixture\n"+source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	ast.Inspect(file, func(node ast.Node) bool {
		found = found || match(node)
		return true
	})
	if found != want {
		t.Errorf("%s: violation=%t, want %t", source, found, want)
	}
}
