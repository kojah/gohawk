package architecture

import (
	"go/ast"
	"go/types"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const internalImportPrefix = "github.com/kojah/gohawk/internal/"

func TestInternalPackagesRespectDependencyDirection(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	for _, source := range inventory.productionGoFiles(
		t,
		"internal/engine/proof",
		"internal/engine/syntax",
		"internal/engine/ssaflow",
		"internal/engine/heapmodel",
		"internal/engine/lifecycle",
		"internal/engine/resourcemodel",
		"internal/analysis/passes",
		"internal/analysis/summaries",
		"internal/reporting/check",
		"internal/analysis/analyzers",
	) {
		from := internalLayer(strings.TrimPrefix(path.Dir(source.repositoryPath), "internal/"))
		for _, imported := range source.file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("%s: parse import %s: %v", source.repositoryPath, imported.Path.Value, err)
			}
			if !strings.HasPrefix(importPath, internalImportPrefix) {
				continue
			}
			toPath := strings.TrimPrefix(importPath, internalImportPrefix)
			to := internalLayer(toPath)
			if forbiddenLayerDependency(from, to) || analyzerImplementationDependency(from, to, path.Dir(source.repositoryPath), toPath) {
				line := source.fileSet.Position(imported.Pos()).Line
				t.Errorf("%s:%d imports %s; %s must not depend on %s", source.repositoryPath, line, importPath, from, to)
			}
		}
	}
}

func internalLayer(packagePath string) string {
	for _, container := range []string{"engine/", "analysis/", "reporting/", "testsupport/"} {
		if nested, ok := strings.CutPrefix(packagePath, container); ok {
			packagePath = nested
			break
		}
	}
	if nested, ok := strings.CutPrefix(packagePath, "ssaflow/"); ok {
		component, _, _ := strings.Cut(nested, "/")
		switch component {
		case "cfg", "calls", "path":
			return component
		}
	}
	component, _, _ := strings.Cut(packagePath, "/")
	switch component {
	case "proof", "syntax", "ssaflow", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace":
		return component
	default:
		return "other"
	}
}

func forbiddenLayerDependency(from, to string) bool {
	switch from {
	case "cfg":
		return to != "proof" && to != "cfg"
	case "calls", "path":
		return from == "calls" && to == "path" || slices.Contains([]string{
			"heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace",
		}, to)
	case "proof":
		return slices.Contains([]string{
			"syntax", "ssaflow", "cfg", "calls", "path", "heapmodel", "lifecycle", "resourcemodel",
			"passes", "summaries", "check", "analyzers", "trace",
		}, to)
	case "syntax":
		return slices.Contains([]string{
			"ssaflow", "cfg", "calls", "path", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers",
		}, to)
	case "ssaflow":
		return slices.Contains([]string{
			"calls", "path", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace",
		}, to)
	case "heapmodel":
		return slices.Contains([]string{"lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "lifecycle":
		return slices.Contains([]string{"resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "resourcemodel":
		return slices.Contains([]string{"passes", "summaries", "check", "analyzers", "trace"}, to)
	case "passes":
		return slices.Contains([]string{"summaries", "check", "analyzers"}, to)
	case "summaries":
		return to == "check" || to == "analyzers"
	case "check":
		return slices.Contains([]string{
			"ssaflow", "cfg", "calls", "path", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "analyzers",
		}, to)
	default:
		return false
	}
}

func TestSemanticModelDependencyBoundaries(t *testing.T) {
	for _, test := range []struct {
		from, to string
		forbid   bool
	}{
		{"lifecycle", "heapmodel", false},
		{"heapmodel", "lifecycle", true},
	} {
		t.Run(test.from+"/"+test.to, func(t *testing.T) {
			if internalLayer(test.from) != test.from || internalLayer(test.to) != test.to {
				t.Fatal("semantic model layer was not recognized")
			}
			if got := forbiddenLayerDependency(test.from, test.to); got != test.forbid {
				t.Fatalf("forbidden dependency = %v, want %v", got, test.forbid)
			}
		})
	}
}

type layerDependencyCase struct {
	from, to string
	forbid   bool
}

func TestSSADependencyBoundaries(t *testing.T) {
	for _, test := range []layerDependencyCase{
		{"ssaflow", "ssaflow/cfg", false},
		{"ssaflow/cfg", "proof", false},
		{"ssaflow/cfg", "ssaflow", true},
		{"ssaflow/cfg", "lifecycle", true},
		{"syntax", "ssaflow/cfg", true},
		{"proof", "ssaflow/cfg", true},
		{"ssaflow/calls", "ssaflow", false},
		{"ssaflow/path", "ssaflow/calls", false},
		{"ssaflow/calls", "ssaflow/path", true},
		{"ssaflow", "ssaflow/calls", true},
		{"ssaflow", "ssaflow/path", true},
		{"ssaflow/cfg", "ssaflow/path", true},
		{"ssaflow/calls", "lifecycle", true},
		{"ssaflow/path", "heapmodel", true},
		{"ssaflow/cfg/cache", "ssaflow/path", true},
		{"check", "ssaflow/cfg", true},
		{"check", "ssaflow/calls", true},
		{"check", "ssaflow/path", true},
		{"engine/ssaflow", "engine/ssaflow/calls", true},
		{"engine/ssaflow/calls", "engine/ssaflow/path", true},
		{"engine/ssaflow/path", "engine/ssaflow/calls", false},
		{"reporting/check", "engine/ssaflow/path", true},
		{"analysis/passes/lifecyclefacts", "analysis/analyzers/resources/resourcelifetime", true},
		{"analysis/summaries", "analysis/passes/lifecyclefacts", false},
	} {
		t.Run(test.from+"/"+test.to, func(t *testing.T) {
			if got := forbiddenLayerDependency(internalLayer(test.from), internalLayer(test.to)); got != test.forbid {
				t.Fatalf("forbidden dependency = %v, want %v", got, test.forbid)
			}
		})
	}
}

func analyzerImplementationDependency(from, to, sourceDirectory, importedPath string) bool {
	if from != "analyzers" || to != "analyzers" {
		return false
	}
	sourcePackage := strings.TrimPrefix(sourceDirectory, "internal/")
	return sourcePackage != importedPath
}

// lifecycleFamilies names the layers inside internal/engine/lifecycle from lowest to
// highest. A file's family is the prefix of its name, and a file may
// reference package-level declarations only from its own family or a lower
// one. The package stays one Go package, so nothing needs exporting to
// cross a family; this test is what keeps the layering real.
var lifecycleFamilies = []string{"store", "completion", "evidence"}

// TestLifecycleFamiliesLayerDownward keeps storage, completion, and evidence
// wrappers layered within lifecycle.
func TestLifecycleFamiliesLayerDownward(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  inventory.root,
	}
	loaded, err := packages.Load(config, "./internal/engine/lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 || len(loaded) != 1 {
		t.Fatalf("load internal/engine/lifecycle: %d packages, %d errors", len(loaded), errors)
	}
	pkg := loaded[0]
	familyOf := make(map[string]int, len(pkg.CompiledGoFiles))
	for _, path := range pkg.CompiledGoFiles {
		family := lifecycleFamily(t, filepath.Base(path))
		familyOf[path] = family
	}
	for index, file := range pkg.Syntax {
		path := pkg.CompiledGoFiles[index]
		using := familyOf[path]
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			object := pkg.TypesInfo.Uses[identifier]
			if object == nil || object.Pkg() != pkg.Types || !packageLevel(object) {
				return true
			}
			defined := pkg.Fset.Position(object.Pos()).Filename
			definedFamily, known := familyOf[defined]
			if !known || definedFamily <= using {
				return true
			}
			t.Errorf("%s (%s) references %s from the higher %s family",
				filepath.Base(path), lifecycleFamilies[using], object.Name(), lifecycleFamilies[definedFamily])
			return true
		})
	}
}

// lifecycleFamily maps a file name to its family index by prefix. doc.go
// belongs to no family and is the lowest so it may be referenced by all.
func lifecycleFamily(t *testing.T, name string) int {
	t.Helper()
	if name == "doc.go" {
		return 0
	}
	prefix, _, _ := strings.Cut(name, "_")
	index := slices.Index(lifecycleFamilies, prefix)
	if index < 0 {
		t.Fatalf("internal/engine/lifecycle/%s does not start with a family prefix (%s)", name, strings.Join(lifecycleFamilies, ", "))
	}
	return index
}

// packageLevel reports whether object is declared at package scope rather
// than as a local, parameter, field, or method.
func packageLevel(object types.Object) bool {
	return object.Parent() != nil && object.Parent().Parent() == types.Universe
}

// TestObjectFactsStayInTheirDefiningPackage enforces that a package may call
// analysis.Pass.ImportObjectFact or ExportObjectFact only for a fact type it
// defines itself.
//
// Fact families are the boundary between intra-procedural evidence discovery
// and cross-package propagation. Each family keeps that propagation inside the
// package that owns the fact type (lifecyclefacts owns lifecycle summaries;
// concurrencyfacts owns ordered synchronization effects), and any analyzer that
// needs a family's conclusions consumes them through that package's facade
// (for example lifecyclefacts.LifecycleEvidence) rather than importing the raw
// fact and re-deriving the call-site argument-to-parameter mapping. Reaching
// into another package's facts by hand is exactly where that mapping gets lost,
// so this test keeps the import/export calls co-located with their definition.
func TestObjectFactsStayInTheirDefiningPackage(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal/analysis/analyzers", "internal/analysis/passes", "internal/analysis/summaries") {
		production[source.absolutePath] = source.repositoryPath
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root,
	}
	loaded, err := packages.Load(config, "./internal/analysis/analyzers/...", "./internal/analysis/passes/...", "./internal/analysis/summaries")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load fact-owning packages: %d errors", errors)
	}
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			if index >= len(pkg.CompiledGoFiles) {
				continue
			}
			repositoryPath, ok := production[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := objectFactCall(pkg.TypesInfo, call)
				if !ok {
					return true
				}
				owner, resolved := factArgumentPackage(pkg.TypesInfo, call)
				if !resolved {
					// The fact argument is not a pointer to a named type we can
					// resolve (for example an interface value). Stay silent
					// rather than invent a boundary violation.
					return true
				}
				if owner == pkg.Types.Path() {
					return true
				}
				position := pkg.Fset.Position(call.Pos())
				t.Errorf("%s:%d calls analysis.Pass.%s for a fact defined in %q; import and export object facts only in the package that defines the fact type",
					repositoryPath, position.Line, method, owner)
				return true
			})
		}
	}
}

// objectFactCall reports whether call invokes analysis.Pass.ImportObjectFact or
// ExportObjectFact, returning the field name. Both are func-typed fields of
// analysis.Pass, so the selector resolves to a *types.Var, not a method.
func objectFactCall(info *types.Info, call *ast.CallExpr) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	field, ok := info.Uses[selector.Sel].(*types.Var)
	if !ok || !field.IsField() || field.Pkg() == nil || field.Pkg().Path() != "golang.org/x/tools/go/analysis" {
		return "", false
	}
	switch field.Name() {
	case "ImportObjectFact", "ExportObjectFact":
		return field.Name(), true
	default:
		return "", false
	}
}

// factArgumentPackage resolves the defining package path of the fact value
// passed to an object-fact call. Both fields take the fact as their second
// argument, and callers pass a pointer to the concrete fact type, so the type
// resolves to a *T whose named element identifies the owning package.
func factArgumentPackage(info *types.Info, call *ast.CallExpr) (string, bool) {
	if len(call.Args) < 2 {
		return "", false
	}
	argument := info.TypeOf(call.Args[1])
	if pointer, ok := argument.(*types.Pointer); ok {
		argument = pointer.Elem()
	}
	named, ok := argument.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return "", false
	}
	return named.Obj().Pkg().Path(), true
}
