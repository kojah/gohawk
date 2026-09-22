package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

type summaryBrokerImporter struct{ pkg *types.Package }

func (importer summaryBrokerImporter) Import(string) (*types.Package, error) {
	return importer.pkg, nil
}

func TestSummaryBrokerMatchesDeclarationIdentity(t *testing.T) {
	for _, test := range []struct {
		name, path, source string
		want               int
	}{
		{"alias", internalImportPrefix + "passes/lifecyclefacts", `package p; import f "domain"; var _ = f.NewLifecycleEvidence`, 1},
		{"dot", internalImportPrefix + "passes/lifecyclefacts", `package p; import . "domain"; var _ = NewLifecycleEvidence`, 1},
		{"prerequisite", internalImportPrefix + "passes/lifecyclefacts", `package p; import f "domain"; var _ = f.Analyzer`, 1},
		{"structure", internalImportPrefix + "passes/lifecyclefacts", `package p; import f "domain"; var _ = f.ResourceCleanup`, 0},
		{"lookalike", "example.org/lifecyclefacts", `package p; import f "domain"; var _ = f.NewLifecycleEvidence`, 0},
		{"literal", internalImportPrefix + "passes/lifecyclefacts", `package p; import f "domain"; var _ = f.Engine{}`, 1},
		{"new", internalImportPrefix + "passes/lifecyclefacts", `package p; import f "domain"; var _ = new(f.Engine)`, 1},
		{"type-alias", internalImportPrefix + "passes/lifecyclefacts", `package p; import f "domain"; type E = f.Engine; var _ = E{}`, 1},
		{"result-assertion", internalImportPrefix + "passes/lifecyclefacts", `package p; import f "domain"; var v any; var _ = v.(*f.Engine)`, 1},
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
