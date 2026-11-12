package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Package layout questions the scheduler asks: which module a file belongs
// to, and which packages of that module import a changed package. The import
// graph comes from `go list`; nothing here type-checks or loads syntax.

// moduleRoot returns the nearest enclosing directory holding a go.mod.
func moduleRoot(directory string) (string, bool) {
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

func insideDirectory(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type listedPackage struct {
	Dir        string   `json:"Dir"`
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
}

// importers returns the directories of every package that imports one of the
// changed directories, directly or through other packages. Facts flow along
// imports, so a changed package can change findings in all of them. With a
// go.work file the search covers every module of the workspace, because an
// importer may live in a sibling module.
func importers(ctx context.Context, guarded *guard, root, overlayFile string, changed []string) ([]string, error) {
	patterns := []string{"./..."}
	for _, module := range workspaceModules(ctx, guarded, root) {
		if module != root {
			patterns = append(patterns, filepath.ToSlash(module)+"/...")
		}
	}
	packages, err := listPackages(ctx, guarded, root, overlayFile, patterns)
	if err != nil {
		return nil, err
	}
	return reverseClosure(packages, changed), nil
}

// workspaceModules lists the main modules the go command sees from root:
// one module, or every module a go.work file names. It returns nil when the
// go command cannot say, and the caller then searches root alone.
func workspaceModules(ctx context.Context, guarded *guard, root string) []string {
	output, err := guarded.run(ctx, root, []string{"list", "-m", "-f", "{{.Dir}}"})
	if err != nil {
		return nil
	}
	var modules []string
	for line := range strings.Lines(string(output.stdout)) {
		if directory := strings.TrimSpace(line); directory != "" {
			modules = append(modules, filepath.Clean(directory))
		}
	}
	return modules
}

func listPackages(ctx context.Context, guarded *guard, root, overlayFile string, patterns []string) ([]listedPackage, error) {
	arguments := []string{"list", "-e", "-json=Dir,ImportPath,Imports"}
	if overlayFile != "" {
		arguments = append(arguments, "-overlay="+overlayFile)
	}
	output, err := guarded.run(ctx, root, append(arguments, patterns...))
	if err != nil {
		return nil, err
	}
	if output.truncated {
		return nil, errOutputTooLarge
	}
	return decodeListedPackages(bytes.NewReader(output.stdout))
}

func decodeListedPackages(input io.Reader) ([]listedPackage, error) {
	var packages []listedPackage
	decoder := json.NewDecoder(input)
	for {
		var next listedPackage
		if err := decoder.Decode(&next); err != nil {
			if errors.Is(err, io.EOF) {
				return packages, nil
			}
			return nil, err
		}
		packages = append(packages, next)
	}
}

func reverseClosure(packages []listedPackage, changed []string) []string {
	directoryOf := make(map[string]string, len(packages))
	importedBy := make(map[string][]string)
	for _, listed := range packages {
		directoryOf[listed.ImportPath] = filepath.Clean(listed.Dir)
		for _, imported := range listed.Imports {
			importedBy[imported] = append(importedBy[imported], listed.ImportPath)
		}
	}
	changedDirectories := make(map[string]bool, len(changed))
	for _, directory := range changed {
		changedDirectories[filepath.Clean(directory)] = true
	}
	var pending []string
	for importPath, directory := range directoryOf {
		if changedDirectories[directory] {
			pending = append(pending, importPath)
		}
	}
	reached := make(map[string]bool)
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, importer := range importedBy[current] {
			if !reached[importer] {
				reached[importer] = true
				pending = append(pending, importer)
			}
		}
	}
	var directories []string
	for importPath := range reached {
		if directory := directoryOf[importPath]; !changedDirectories[directory] {
			directories = append(directories, directory)
		}
	}
	return directories
}

// relativePatterns names package directories the way the go command expects
// them from the module root: relative when inside it, absolute otherwise.
func relativePatterns(root string, directories []string) []string {
	patterns := make([]string, 0, len(directories))
	for _, directory := range directories {
		relative, err := filepath.Rel(root, directory)
		if err != nil || !filepath.IsLocal(relative) {
			// A package of a sibling workspace module is named by its
			// absolute directory.
			patterns = append(patterns, directory)
			continue
		}
		patterns = append(patterns, "./"+filepath.ToSlash(relative))
	}
	sort.Strings(patterns)
	return patterns
}
