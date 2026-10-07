package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// repositorySourceInventory gives architecture tests one stable view of
// maintained Go source. Authored views exclude generated files, while layout
// counts include them. All views share fixture and external-tree exclusions.
type repositorySourceInventory struct {
	root string
}

type sourceView uint8

const (
	authoredProduction sourceView = iota
	authoredIncludingTests
	productionLayout
	testLayout
)

// includesFile reports whether a view covers a Go file. Only the authored view
// mixes production and test files; layout views count each kind on its own.
func (view sourceView) includesFile(path string) bool {
	isTest := strings.HasSuffix(path, "_test.go")
	switch view {
	case authoredIncludingTests:
		return true
	case testLayout:
		return isTest
	default:
		return !isTest
	}
}

// countsGenerated reports whether generated files belong to the view. Layout
// views count every file in a directory; authored views skip generated code.
func (view sourceView) countsGenerated() bool {
	return view == productionLayout || view == testLayout
}

type repositoryGoSource struct {
	absolutePath   string
	repositoryPath string
	source         []byte
	fileSet        *token.FileSet
	file           *ast.File
}

func newRepositorySourceInventory(t *testing.T) repositorySourceInventory {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture source inventory")
	}
	root, err := findRepositoryRoot(filepath.Dir(currentFile))
	if err != nil {
		t.Fatal(err)
	}
	return repositorySourceInventory{root: root}
}

func findRepositoryRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
		current = resolved
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil {
			return filepath.Clean(current), nil
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fs.ErrNotExist
		}
		current = parent
	}
}

func (inventory repositorySourceInventory) productionGoFiles(t *testing.T, roots ...string) []repositoryGoSource {
	t.Helper()
	return inventory.goFiles(t, authoredProduction, roots...)
}

// authoredGoFiles includes tests for invariants that apply to all maintained Go
// code. Fixture, generated and external trees retain the shared exclusions.
func (inventory repositorySourceInventory) authoredGoFiles(t *testing.T, roots ...string) []repositoryGoSource {
	t.Helper()
	return inventory.goFiles(t, authoredIncludingTests, roots...)
}

// layoutGoFiles counts all production source, including generated files and
// inactive build variants. Layout is a repository invariant, not a build view.
func (inventory repositorySourceInventory) layoutGoFiles(t *testing.T, roots ...string) []repositoryGoSource {
	t.Helper()
	return inventory.goFiles(t, productionLayout, roots...)
}

// testLayoutGoFiles counts all test source, including generated tests. Like
// production layout, it measures how many files a directory holds.
func (inventory repositorySourceInventory) testLayoutGoFiles(t *testing.T, roots ...string) []repositoryGoSource {
	t.Helper()
	return inventory.goFiles(t, testLayout, roots...)
}

func (inventory repositorySourceInventory) goFiles(t *testing.T, view sourceView, roots ...string) []repositoryGoSource {
	t.Helper()
	files := make(map[string]repositoryGoSource)
	for _, root := range roots {
		scope := inventory.scopedRoot(t, root)
		err := filepath.WalkDir(scope, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if excludedSourceDirectory(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".go" || !view.includesFile(path) {
				return nil
			}
			source, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			fileSet := token.NewFileSet()
			file, parseErr := parser.ParseFile(fileSet, path, source, parser.ParseComments)
			if parseErr != nil {
				return parseErr
			}
			if !view.countsGenerated() && ast.IsGenerated(file) {
				return nil
			}
			repositoryPath, relativeErr := filepath.Rel(inventory.root, path)
			if relativeErr != nil {
				return relativeErr
			}
			repositoryPath = filepath.ToSlash(repositoryPath)
			files[repositoryPath] = repositoryGoSource{
				absolutePath:   stableAbsolutePath(path),
				repositoryPath: repositoryPath,
				source:         source,
				fileSet:        fileSet,
				file:           file,
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	result := make([]repositoryGoSource, 0, len(paths))
	for _, path := range paths {
		result = append(result, files[path])
	}
	return result
}

func stableAbsolutePath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute)
}

func (inventory repositorySourceInventory) scopedRoot(t *testing.T, root string) string {
	t.Helper()
	if filepath.IsAbs(root) {
		t.Fatalf("source inventory root %q must be repository-relative", root)
	}
	scope := filepath.Clean(filepath.Join(inventory.root, filepath.FromSlash(root)))
	relative, err := filepath.Rel(inventory.root, scope)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("source inventory root %q escapes repository", root)
	}
	return scope
}

func excludedSourceDirectory(name string) bool {
	// Like Go package discovery, ignore hidden and underscore-prefixed trees.
	// In particular, .build contains audit checkouts, not our production source.
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	switch name {
	case "fixture", "fixtures", "testdata", "vendor", "node_modules":
		return true
	default:
		return false
	}
}

func TestSourceInventoryExcludesNonProductionTrees(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := map[string]string{
		"main.go":                      "package main\n",
		"internal/worker/work.go":      "package worker\n",
		"internal/worker/work_test.go": "not Go source",
		"internal/worker/generated.go": "// Code generated by test. DO NOT EDIT.\npackage worker\n",
	}
	for _, directory := range []string{".build/audit/checkouts/template", ".cache", "_scratch", "fixture", "fixtures", "testdata", "vendor"} {
		files[directory+"/invalid.go"] = "not Go source"
	}
	for name, source := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inventory := repositorySourceInventory{root: root}
	var got []string
	for _, source := range inventory.productionGoFiles(t, ".", "internal") {
		got = append(got, source.repositoryPath)
	}
	want := []string{"internal/worker/work.go", "main.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("production source = %v, want %v", got, want)
	}
}

func TestSourceInventoryIncludesAuthoredTests(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, source := range map[string]string{
		"main.go":              "package main\n",
		"main_test.go":         "package main\n",
		"generated_test.go":    "// Code generated by test. DO NOT EDIT.\npackage main\n",
		"testdata/bad_test.go": "not Go source",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inventory := repositorySourceInventory{root: root}
	var got []string
	for _, source := range inventory.authoredGoFiles(t, ".", ".") {
		got = append(got, source.repositoryPath)
	}
	want := []string{"main.go", "main_test.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("authored source = %v, want %v", got, want)
	}
}
