package architecture

import (
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestDocumentationReferencesResolve keeps the development documentation, the
// contributor guides, AGENTS.md, and the project-local skills in sync with the
// code they describe. Two things can rot independently, so the test checks
// both directions.
//
// Prose that cites code must cite code that exists. Every package-qualified
// reference such as `check.Report`, `ssaflow.WalkStates`, or
// `analysis.Pass.Report` is resolved against the type-checked scope of that
// package, so a renamed helper fails the build instead of leaving a paragraph
// pointing at nothing. A qualifier that is not a known package (`t.Helper`,
// where t is a variable) is skipped rather than guessed at. Test names and
// `make` targets are resolved the same way. Bare identifiers are resolved only
// in the generated helper index, where every code span names a helper; other
// pages use identifiers from many packages in ordinary prose, and flagging a
// bare `Close` there would cry wolf.
//
// The inventories those pages promise to be complete must be complete: every
// exported ssaflow and lifecyclefacts function appears in the generated helper
// index, every Fact field appears in the fact-model page, and every
// architecture test appears in the invariants table of the architecture
// guide, so a new helper or test cannot be added without being documented.
func TestDocumentationReferencesResolve(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	symbols := loadDocumentedSymbols(t, inventory.root)
	tests := architectureTestNames(t, inventory.root)
	targets := makefileTargets(t, inventory.root)

	for _, page := range documentationPages(t, inventory.root) {
		checkPageReferences(t, page, symbols, tests, targets)
	}
	checkInventoryCoverage(t, inventory.root, symbols, tests)
}

// documentedPackagePatterns are the packages whose identifiers the
// documentation may cite with a package qualifier: every repository package
// plus the external packages the guides name when explaining the analysis
// model. A qualifier outside this set is not an error; it is simply not
// resolved.
var documentedPackagePatterns = []string{
	"./internal/...",
	"./analyzers",
	"golang.org/x/tools/go/analysis",
	"golang.org/x/tools/go/ssa",
	"os",
	"log",
}

// inventoryPackages are the packages whose exported functions the generated
// helper index promises to list completely, and against which bare
// identifiers in that index are resolved.
var inventoryPackages = []string{"ssaflow", "lifecyclefacts"}

// documentedSymbols is the exported surface of the packages the documentation
// may cite, indexed by package name.
type documentedSymbols struct {
	// names holds every exported package-level identifier, by package name.
	names map[string]map[string]bool
	// members holds exported methods and struct fields, by package and type.
	members map[string]map[string]map[string]bool
	// functions holds exported package-level functions, by package, for the
	// coverage check.
	functions map[string][]string
	// factFields holds the fields of lifecyclefacts.Fact in declaration order.
	factFields []string
}

func loadDocumentedSymbols(t *testing.T, root string) *documentedSymbols {
	t.Helper()
	config := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: root}
	loaded, err := packages.Load(config, documentedPackagePatterns...)
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load documented packages: %d errors", errors)
	}
	symbols := &documentedSymbols{
		names:     map[string]map[string]bool{},
		members:   map[string]map[string]map[string]bool{},
		functions: map[string][]string{},
	}
	for _, pkg := range loaded {
		// A qualifier must denote one package, or resolution would be a guess.
		if _, seen := symbols.names[pkg.Types.Name()]; seen {
			t.Fatalf("two documented packages are named %q; qualified references would be ambiguous", pkg.Types.Name())
		}
		symbols.addPackage(pkg.Types)
	}
	return symbols
}

func (symbols *documentedSymbols) addPackage(pkg *types.Package) {
	name := pkg.Name()
	symbols.names[name] = map[string]bool{}
	symbols.members[name] = map[string]map[string]bool{}
	scope := pkg.Scope()
	for _, identifier := range scope.Names() {
		object := scope.Lookup(identifier)
		if !object.Exported() {
			continue
		}
		symbols.names[name][identifier] = true
		if _, ok := object.(*types.Func); ok {
			symbols.functions[name] = append(symbols.functions[name], identifier)
			continue
		}
		typeName, ok := object.(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := typeName.Type().(*types.Named)
		if !ok {
			continue
		}
		symbols.members[name][identifier] = symbols.addMembers(name, identifier, named)
	}
	slices.Sort(symbols.functions[name])
}

func (symbols *documentedSymbols) addMembers(pkg, typeName string, named *types.Named) map[string]bool {
	members := map[string]bool{}
	for method := range named.Methods() {
		if method.Exported() {
			members[method.Name()] = true
		}
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok {
		return members
	}
	for field := range structure.Fields() {
		if !field.Exported() {
			continue
		}
		members[field.Name()] = true
		if pkg == "lifecyclefacts" && typeName == "Fact" {
			symbols.factFields = append(symbols.factFields, field.Name())
		}
	}
	return members
}

// knownPackage reports whether qualifier names a documented package.
func (symbols *documentedSymbols) knownPackage(qualifier string) bool {
	_, ok := symbols.names[qualifier]
	return ok
}

// resolves reports whether name, or some identifier with that prefix, is an
// exported package-level identifier of pkg.
func (symbols *documentedSymbols) resolves(pkg, name string, prefix bool) bool {
	names := symbols.names[pkg]
	if !prefix {
		return names[name]
	}
	for candidate := range names {
		if strings.HasPrefix(candidate, name) {
			return true
		}
	}
	return false
}

// resolvesMember reports whether member is an exported method or field of
// the named type in pkg.
func (symbols *documentedSymbols) resolvesMember(pkg, typeName, member string) bool {
	return symbols.members[pkg][typeName][member]
}

// resolvesBare reports whether an unqualified identifier names an exported
// identifier, method, or field in one of the inventory packages.
func (symbols *documentedSymbols) resolvesBare(name, member string, prefix bool) bool {
	for _, pkg := range inventoryPackages {
		if member != "" {
			if symbols.resolvesMember(pkg, name, member) {
				return true
			}
			continue
		}
		if symbols.resolves(pkg, name, prefix) {
			return true
		}
		for _, members := range symbols.members[pkg] {
			if members[name] {
				return true
			}
		}
	}
	return false
}

func architectureTestNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "internal", "architecture", "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)
	names := map[string]bool{}
	for _, file := range files {
		for _, match := range pattern.FindAllStringSubmatch(readFile(t, file), -1) {
			names[match[1]] = true
		}
	}
	return names
}

func makefileTargets(t *testing.T, root string) map[string]bool {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z0-9_-]*):`)
	targets := map[string]bool{}
	for _, match := range pattern.FindAllStringSubmatch(readFile(t, filepath.Join(root, "Makefile")), -1) {
		targets[match[1]] = true
	}
	return targets
}

// documentationPage is one file whose code references the test resolves.
type documentationPage struct {
	relative string
	text     string
	// helperIndex marks the generated helper index, where every bare code span
	// names a helper and is therefore resolved.
	helperIndex bool
}

func documentationPages(t *testing.T, root string) []documentationPage {
	t.Helper()
	paths := []string{
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, "docs", "architecture.md"),
		filepath.Join(root, "docs", "contributing.md"),
	}
	development, err := filepath.Glob(filepath.Join(root, "docs", "development", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, development...)
	skills, err := filepath.Glob(filepath.Join(root, ".agents", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range skills {
		if projectLocalSkill(readFile(t, skill)) {
			paths = append(paths, skill)
		}
	}
	paths = append(paths, filepath.Join(root, filepath.FromSlash(helperIndexPage)))
	pages := make([]documentationPage, 0, len(paths))
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, documentationPage{
			relative:    filepath.ToSlash(relative),
			text:        readFile(t, path),
			helperIndex: filepath.ToSlash(relative) == helperIndexPage,
		})
	}
	return pages
}

// helperIndexPage is the generated inventory of every exported helper. It
// lives with the codebase skill because it is searched by name rather than
// read; the website page keeps the curated map from questions to helpers.
const helperIndexPage = ".agents/skills/gohawk-codebase/references/shared-helpers.md"

// projectLocalSkill reports whether a skill declares `source: project` in its
// front matter. Vault-managed skills describe other codebases and are not
// checked against this one.
func projectLocalSkill(text string) bool {
	if !strings.HasPrefix(text, "---\n") {
		return false
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return false
	}
	return strings.Contains(text[4:4+end], "source: project")
}

var (
	codeSpan = regexp.MustCompile("`([^`\n]+)`")
	// qualifiedReference matches pkg.Name or pkg.Type.Member, with an optional
	// leading pointer star, type-parameter list, or call suffix, and a
	// trailing * that means "every identifier with this prefix".
	qualifiedReference = regexp.MustCompile(
		`^\*?([a-z][a-z0-9]*)\.([A-Z][A-Za-z0-9_]*)(?:\[[^\]]*\])?(?:\.([A-Z][A-Za-z0-9_]*))?(?:\([^()]*\))?(\*?)$`)
	bareReference = regexp.MustCompile(
		`^([A-Z][A-Za-z0-9_]*)(?:\[[^\]]*\])?(?:\.([A-Z][A-Za-z0-9_]*))?(?:\([^()]*\))?(\*?)$`)
	testReference = regexp.MustCompile(`^Test[A-Z][A-Za-z0-9_]*$`)
	makeReference = regexp.MustCompile(`^make ([a-z][a-z0-9-]*)$`)
)

func checkPageReferences(t *testing.T, page documentationPage, symbols *documentedSymbols, tests, targets map[string]bool) {
	t.Helper()
	for _, match := range codeSpan.FindAllStringSubmatchIndex(page.text, -1) {
		token := page.text[match[2]:match[3]]
		line := 1 + strings.Count(page.text[:match[2]], "\n")
		switch {
		case testReference.MatchString(token):
			if !tests[token] {
				t.Errorf("%s:%d cites %s, which is not a test in internal/architecture", page.relative, line, token)
			}
		case makeReference.MatchString(token):
			target := makeReference.FindStringSubmatch(token)[1]
			if !targets[target] {
				t.Errorf("%s:%d cites `make %s`, which is not a Makefile target", page.relative, line, target)
			}
		case qualifiedReference.MatchString(token):
			checkQualifiedReference(t, page, line, token, symbols)
		case page.helperIndex && bareReference.MatchString(token):
			parts := bareReference.FindStringSubmatch(token)
			if !symbols.resolvesBare(parts[1], parts[2], parts[3] == "*") {
				t.Errorf("%s:%d cites `%s`, which is not an exported ssaflow or lifecyclefacts identifier", page.relative, line, token)
			}
		}
	}
}

func checkQualifiedReference(t *testing.T, page documentationPage, line int, token string, symbols *documentedSymbols) {
	t.Helper()
	parts := qualifiedReference.FindStringSubmatch(token)
	pkg, name, member, prefix := parts[1], parts[2], parts[3], parts[4] == "*"
	if !symbols.knownPackage(pkg) {
		// A lowercase qualifier that is not a package is a variable such as
		// `t` or `pass`; there is nothing to resolve it against.
		return
	}
	if !symbols.resolves(pkg, name, prefix) {
		t.Errorf("%s:%d cites `%s`, but package %s has no exported %s", page.relative, line, token, pkg, name)
		return
	}
	if member != "" && !symbols.resolvesMember(pkg, name, member) {
		t.Errorf("%s:%d cites `%s`, but %s.%s has no exported %s", page.relative, line, token, pkg, name, member)
	}
}

// checkInventoryCoverage enforces the completeness each inventory page
// promises, so additions to the code cannot go undocumented.
func checkInventoryCoverage(t *testing.T, root string, symbols *documentedSymbols, tests map[string]bool) {
	t.Helper()
	helperIndex := readFile(t, filepath.Join(root, filepath.FromSlash(helperIndexPage)))
	for _, pkg := range inventoryPackages {
		for _, function := range symbols.functions[pkg] {
			if !mentionsIdentifier(helperIndex, function) {
				t.Errorf("%s does not document %s.%s", helperIndexPage, pkg, function)
			}
		}
	}
	factModel := readFile(t, filepath.Join(root, "docs", "development", "fact-model.md"))
	for _, field := range symbols.factFields {
		if !mentionsIdentifier(factModel, field) {
			t.Errorf("docs/development/fact-model.md does not document the Fact field %s", field)
		}
	}
	architecture := readFile(t, filepath.Join(root, "docs", "architecture.md"))
	for _, test := range slices.Sorted(maps.Keys(tests)) {
		if !mentionsIdentifier(architecture, test) {
			t.Errorf("docs/architecture.md invariants table does not list %s", test)
		}
	}
}

func mentionsIdentifier(text, identifier string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(identifier) + `\b`).MatchString(text)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
