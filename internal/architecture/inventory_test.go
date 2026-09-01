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
// production Go source. Test fixtures, generated files, and tests are excluded
// here so individual invariants do not grow subtly different walkers.
type repositorySourceInventory struct {
	root string
}

type productionGoSource struct {
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

func (inventory repositorySourceInventory) productionGoFiles(t *testing.T, roots ...string) []productionGoSource {
	t.Helper()
	files := make(map[string]productionGoSource)
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
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
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
			if ast.IsGenerated(file) {
				return nil
			}
			repositoryPath, relativeErr := filepath.Rel(inventory.root, path)
			if relativeErr != nil {
				return relativeErr
			}
			repositoryPath = filepath.ToSlash(repositoryPath)
			files[repositoryPath] = productionGoSource{
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
	result := make([]productionGoSource, 0, len(paths))
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
	switch name {
	case ".git", "fixture", "fixtures", "testdata", "vendor":
		return true
	default:
		return false
	}
}
