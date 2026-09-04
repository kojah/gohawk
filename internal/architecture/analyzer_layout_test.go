package architecture

import (
	"go/ast"
	"go/types"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	publicanalyzers "github.com/kojah/gohawk/analyzers"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

type analyzerLayoutPackage struct {
	pkg   *packages.Package
	group string
	name  string
	dir   string
}

func TestAnalyzerPackageLayout(t *testing.T) {
	inventory := newRepositorySourceInventory(t)
	packages, catalogPackage, modulePath, loaded := loadAnalyzerLayoutPackages(t, inventory)
	assertAnalyzerSourceDepth(t, inventory, packages)
	assertInfrastructureAnalyzerPlacement(t, loaded, modulePath)
	withdrawn := withdrawnAnalyzerNames()
	runtimeAnalyzers := assertAnalyzerRuntimeBijection(t, packages, withdrawn)
	assertCatalogFactories(t, inventory, catalogPackage, modulePath, packages)
	for _, analyzerPackage := range packages {
		assertLocalTestdata(t, analyzerPackage)
		if withdrawn[analyzerPackage.name] {
			// A withdrawn analyzer has no runtime entry to compare against, so
			// its prerequisites are checked by its own package tests instead.
			continue
		}
		prerequisites := 0
		if analyzer := runtimeAnalyzers[analyzerPackage.name]; analyzer != nil {
			prerequisites = len(analyzer.Requires)
		}
		assertPrerequisitePlacement(t, analyzerPackage, modulePath, prerequisites)
	}
}

func loadAnalyzerLayoutPackages(
	t *testing.T,
	inventory repositorySourceInventory,
) ([]analyzerLayoutPackage, *packages.Package, string, []*packages.Package) {
	t.Helper()
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root,
	}
	loaded, err := packages.Load(config, "./analyzers", "./internal/...")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load analyzer layout packages: %d errors", errors)
	}
	var catalogPackage *packages.Package
	for _, pkg := range loaded {
		if strings.HasSuffix(pkg.PkgPath, "/analyzers") && !strings.Contains(pkg.PkgPath, "/internal/analyzers/") {
			catalogPackage = pkg
			break
		}
	}
	if catalogPackage == nil {
		t.Fatal("public analyzers package was not loaded")
	}
	modulePath := strings.TrimSuffix(catalogPackage.PkgPath, "/analyzers")
	prefix := modulePath + "/internal/analyzers/"
	result := make([]analyzerLayoutPackage, 0)
	for _, pkg := range loaded {
		relative, internalAnalyzer := strings.CutPrefix(pkg.PkgPath, prefix)
		if !internalAnalyzer {
			continue
		}
		parts := strings.Split(relative, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			t.Errorf("%s must be exactly internal/analyzers/<group>/<name>; group directories must not be Go packages", pkg.PkgPath)
			continue
		}
		if pkg.Name != parts[1] {
			t.Errorf("%s package name = %q, want path name %q", pkg.PkgPath, pkg.Name, parts[1])
		}
		assertAnalyzerFactory(t, pkg)
		if len(pkg.CompiledGoFiles) == 0 {
			t.Errorf("%s has no production Go files", pkg.PkgPath)
			continue
		}
		result = append(result, analyzerLayoutPackage{
			pkg: pkg, group: parts[0], name: parts[1], dir: filepath.Dir(stableAbsolutePath(pkg.CompiledGoFiles[0])),
		})
	}
	return result, catalogPackage, modulePath, loaded
}

func assertAnalyzerSourceDepth(t *testing.T, inventory repositorySourceInventory, layout []analyzerLayoutPackage) {
	t.Helper()
	known := make(map[string]bool, len(layout))
	for _, analyzerPackage := range layout {
		known[analyzerPackage.group+"/"+analyzerPackage.name] = true
	}
	for _, source := range inventory.productionGoFiles(t, "internal/analyzers") {
		relative := strings.TrimPrefix(source.repositoryPath, "internal/analyzers/")
		parts := strings.Split(relative, "/")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
			t.Errorf("%s must be directly inside internal/analyzers/<group>/<name>", source.repositoryPath)
			continue
		}
		identity := parts[0] + "/" + parts[1]
		if !known[identity] {
			t.Errorf("%s has no active analyzer package or runtime registration", source.repositoryPath)
		}
		if source.file.Name.Name != parts[1] {
			t.Errorf("%s package name = %q, want %q", source.repositoryPath, source.file.Name.Name, parts[1])
		}
	}
}

func assertInfrastructureAnalyzerPlacement(t *testing.T, loaded []*packages.Package, modulePath string) {
	t.Helper()
	passesPrefix := modulePath + "/internal/passes/"
	for _, pkg := range loaded {
		object := pkg.Types.Scope().Lookup("Analyzer")
		variable, analyzerVariable := object.(*types.Var)
		if analyzerVariable && analysisAnalyzerPointer(variable.Type()) && !strings.HasPrefix(pkg.PkgPath, passesPrefix) {
			t.Errorf("%s exports an Analyzer variable outside internal/passes/<name>", pkg.PkgPath)
		}
		relative, passPackage := strings.CutPrefix(pkg.PkgPath, passesPrefix)
		if !passPackage {
			continue
		}
		if relative == "" || strings.Contains(relative, "/") {
			t.Errorf("%s prerequisite pass must be exactly internal/passes/<name>", pkg.PkgPath)
		}
		if !analyzerVariable || !analysisAnalyzerPointer(variable.Type()) {
			t.Errorf("%s must export an Analyzer variable of type *analysis.Analyzer", pkg.PkgPath)
		}
	}
}

func assertAnalyzerFactory(t *testing.T, pkg *packages.Package) {
	t.Helper()
	object := pkg.Types.Scope().Lookup("Analyzer")
	factory, ok := object.(*types.Func)
	if !ok || !factory.Exported() {
		t.Errorf("%s must export an Analyzer factory function", pkg.PkgPath)
		return
	}
	signature, ok := factory.Type().(*types.Signature)
	if !ok || signature.Recv() != nil || signature.Results().Len() != 1 || !analysisAnalyzerPointer(signature.Results().At(0).Type()) {
		t.Errorf("%s Analyzer must be a function returning exactly *analysis.Analyzer", pkg.PkgPath)
	}
}

func analysisAnalyzerPointer(value types.Type) bool {
	pointer, ok := value.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := pointer.Elem().(*types.Named)
	return ok && named.Obj().Name() == "Analyzer" && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "golang.org/x/tools/go/analysis"
}

// withdrawnAnalyzerPackages are analyzers whose checks are all delisted in
// analyzers/catalog_specs.go. Their packages and tests remain, so the
// package-to-catalog bijection has to know they are deliberately absent rather
// than accidentally unregistered. See
// https://github.com/kojah/gohawk/issues/34.
//
// The list is checked in both directions: a package here that turns out to be
// registered fails, so relisting an analyzer cannot leave a stale entry behind.
var withdrawnAnalyzerPackages = map[string]bool{
	"apishape":      true,
	"contextpolicy": true,
	"closedomain":   true,
	"wirepolicy":    true,
	"taintpolicy":   true,
	"testlifecycle": true,
	"testpolicy":    true,
}

func withdrawnAnalyzerNames() map[string]bool {
	return withdrawnAnalyzerPackages
}

func assertAnalyzerRuntimeBijection(t *testing.T, layout []analyzerLayoutPackage, withdrawn map[string]bool) map[string]*analysis.Analyzer {
	t.Helper()
	byName := make(map[string]analyzerLayoutPackage, len(layout))
	for _, analyzerPackage := range layout {
		if previous, exists := byName[analyzerPackage.name]; exists {
			t.Errorf("analyzer name %q belongs to both groups %q and %q", analyzerPackage.name, previous.group, analyzerPackage.group)
		}
		byName[analyzerPackage.name] = analyzerPackage
	}
	grouped := make(map[string]bool, len(layout))
	runtimeAnalyzers := make(map[string]*analysis.Analyzer, len(layout))
	seenGroups := make(map[string]bool)
	for _, group := range publicanalyzers.AnalyzerGroups() {
		if seenGroups[group.Name] {
			t.Errorf("runtime analyzer group %q appears more than once", group.Name)
		}
		seenGroups[group.Name] = true
		if len(group.Analyzers) == 0 {
			t.Errorf("runtime analyzer group %q is empty", group.Name)
		}
		for _, analyzer := range group.Analyzers {
			if analyzer == nil {
				t.Errorf("runtime analyzer group %q contains a nil analyzer", group.Name)
				continue
			}
			owner, exists := byName[analyzer.Name]
			if !exists {
				t.Errorf("runtime analyzer %q has no internal analyzer package", analyzer.Name)
				continue
			}
			if grouped[analyzer.Name] {
				t.Errorf("runtime analyzer %q appears in more than one group", analyzer.Name)
			}
			grouped[analyzer.Name] = true
			runtimeAnalyzers[analyzer.Name] = analyzer
			if group.Name != owner.group || analyzer.Name != owner.name || owner.pkg.Name != owner.name {
				t.Errorf("runtime analyzer %q group/path/package = %q/%q/%q, want exact agreement", analyzer.Name, group.Name, owner.name, owner.pkg.Name)
			}
		}
	}
	metadata := publicanalyzers.AnalyzerMetadata()
	for name := range byName {
		if withdrawn[name] {
			if grouped[name] {
				t.Errorf("analyzer %q is registered but listed as withdrawn; remove it from withdrawnAnalyzerPackages", name)
			}
			continue
		}
		if !grouped[name] {
			t.Errorf("analyzer package %q is absent from AnalyzerGroups", name)
		}
		if _, exists := metadata[name]; !exists {
			t.Errorf("analyzer package %q is absent from AnalyzerMetadata", name)
		}
	}
	for name := range metadata {
		if _, exists := byName[name]; !exists {
			t.Errorf("AnalyzerMetadata contains %q without an analyzer package", name)
		}
		if !grouped[name] {
			t.Errorf("AnalyzerMetadata contains %q without AnalyzerGroups registration", name)
		}
	}
	return runtimeAnalyzers
}

func assertCatalogFactories(
	t *testing.T,
	inventory repositorySourceInventory,
	catalogPackage *packages.Package,
	modulePath string,
	layout []analyzerLayoutPackage,
) {
	t.Helper()
	production := make(map[string]bool)
	for _, source := range inventory.productionGoFiles(t, "analyzers") {
		production[source.absolutePath] = true
	}
	counts := make(map[string]int, len(layout))
	known := make(map[string]bool, len(layout))
	for _, analyzerPackage := range layout {
		known[analyzerPackage.pkg.PkgPath] = true
	}
	for index, file := range catalogPackage.Syntax {
		if index >= len(catalogPackage.CompiledGoFiles) || !production[stableAbsolutePath(catalogPackage.CompiledGoFiles[index])] {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			factory := calledFunction(catalogPackage.TypesInfo, call.Fun)
			if factory == nil || factory.Name() != "Analyzer" || !functionReturnsAnalyzer(factory) || factory.Pkg() == nil {
				return true
			}
			path := factory.Pkg().Path()
			if strings.HasPrefix(path, modulePath+"/internal/analyzers/") && !known[path] {
				t.Errorf("catalog calls unrecognized analyzer factory %s.Analyzer", path)
				return true
			}
			if known[path] {
				counts[path]++
			}
			return true
		})
	}
	for _, analyzerPackage := range layout {
		if counts[analyzerPackage.pkg.PkgPath] != 1 {
			t.Errorf("catalog calls %s.Analyzer %d times, want exactly once", analyzerPackage.pkg.PkgPath, counts[analyzerPackage.pkg.PkgPath])
		}
	}
}

func calledFunction(info *types.Info, expression ast.Expr) *types.Func {
	switch typed := expression.(type) {
	case *ast.Ident:
		function, _ := info.Uses[typed].(*types.Func)
		return function
	case *ast.SelectorExpr:
		function, _ := info.Uses[typed.Sel].(*types.Func)
		return function
	case *ast.IndexExpr:
		return calledFunction(info, typed.X)
	case *ast.IndexListExpr:
		return calledFunction(info, typed.X)
	default:
		return nil
	}
}

func functionReturnsAnalyzer(function *types.Func) bool {
	signature, ok := function.Type().(*types.Signature)
	return ok && signature.Results().Len() == 1 && analysisAnalyzerPointer(signature.Results().At(0).Type())
}

func assertLocalTestdata(t *testing.T, analyzerPackage analyzerLayoutPackage) {
	t.Helper()
	testdata := stableAbsolutePath(filepath.Join(analyzerPackage.dir, "testdata"))
	owned := false
	fixtureOwned := false
	fixtureRoot := stableAbsolutePath(filepath.Join(testdata, "src", analyzerPackage.name))
	err := filepath.WalkDir(testdata, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			resolved := stableAbsolutePath(path)
			if !pathWithin(testdata, resolved) {
				return &fs.PathError{Op: "fixture ancestry", Path: path, Err: fs.ErrInvalid}
			}
			owned = true
			fixtureOwned = fixtureOwned || pathWithin(fixtureRoot, resolved)
		}
		return nil
	})
	switch {
	case err != nil:
		t.Errorf("%s testdata: %v", analyzerPackage.pkg.PkgPath, err)
	case !owned:
		t.Errorf("%s must own non-empty local testdata", analyzerPackage.pkg.PkgPath)
	case !fixtureOwned:
		t.Errorf("%s must own fixtures below testdata/src/%s", analyzerPackage.pkg.PkgPath, analyzerPackage.name)
	}
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func assertPrerequisitePlacement(t *testing.T, analyzerPackage analyzerLayoutPackage, modulePath string, runtimeCount int) {
	t.Helper()
	sourceCount := 0
	for _, file := range analyzerPackage.pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			field, ok := node.(*ast.KeyValueExpr)
			if !ok || !analysisRequiresField(analyzerPackage.pkg.TypesInfo, field.Key) {
				return true
			}
			literal, ok := field.Value.(*ast.CompositeLit)
			if !ok {
				t.Errorf("%s Requires must list prerequisite analyzers directly", analyzerPackage.pkg.PkgPath)
				return false
			}
			for _, element := range literal.Elts {
				sourceCount++
				provider := expressionObject(analyzerPackage.pkg.TypesInfo, element)
				if provider == nil || provider.Pkg() == nil || !analysisAnalyzerPointer(provider.Type()) {
					t.Errorf("%s has an unresolved Requires entry", analyzerPackage.pkg.PkgPath)
					continue
				}
				if !prerequisitePackage(provider.Pkg().Path(), modulePath) {
					t.Errorf(
						"%s Requires entry %s must come from internal/passes or x/tools analysis/passes",
						analyzerPackage.pkg.PkgPath,
						provider.Pkg().Path(),
					)
				}
			}
			return false
		})
	}
	// FactTypes make closedomain a fact-producing catalog analyzer, not an
	// infrastructure prerequisite. Only the typed Requires field contributes to
	// this count, which keeps those two analysis.Analyzer fields distinct.
	if sourceCount != runtimeCount {
		t.Errorf(
			"%s declares %d typed Requires entries, runtime analyzer has %d",
			analyzerPackage.pkg.PkgPath,
			sourceCount,
			runtimeCount,
		)
	}
}

func analysisRequiresField(info *types.Info, expression ast.Expr) bool {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	field, ok := info.Uses[identifier].(*types.Var)
	return ok && field.IsField() && field.Name() == "Requires" && field.Pkg() != nil &&
		field.Pkg().Path() == "golang.org/x/tools/go/analysis"
}

func expressionObject(info *types.Info, expression ast.Expr) types.Object {
	switch typed := expression.(type) {
	case *ast.Ident:
		return info.Uses[typed]
	case *ast.SelectorExpr:
		return info.Uses[typed.Sel]
	case *ast.CallExpr:
		return calledFunction(info, typed.Fun)
	case *ast.ParenExpr:
		return expressionObject(info, typed.X)
	default:
		return nil
	}
}

func prerequisitePackage(packagePath, modulePath string) bool {
	for _, prefix := range []string{"golang.org/x/tools/go/analysis/passes/", modulePath + "/internal/passes/"} {
		if relative, ok := strings.CutPrefix(packagePath, prefix); ok {
			return relative != "" && !strings.Contains(relative, "/")
		}
	}
	return false
}
