package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func synchronize(root string, check bool) error {
	data, err := collectManifest(root)
	if err != nil {
		return err
	}

	expectedPages := make(map[string]bool)
	updates := make(map[string][]byte)
	for _, group := range data.Groups {
		for _, analyzer := range group.Analyzers {
			page := filepath.Join(root, "docs", filepath.FromSlash(analyzer.Path+".mdx"))
			expectedPages[page] = true
			contents, err := os.ReadFile(page)
			if err != nil {
				return fmt.Errorf("analyzer %q has no documentation page at %s", analyzer.Name, relativePath(root, page))
			}
			if !hasFrontmatterTitle(contents, analyzer.Name) {
				return fmt.Errorf("%s must have frontmatter title %q", relativePath(root, page), analyzer.Name)
			}
			contents, err = synchronizeAnalyzerComponents(contents)
			if err != nil {
				return fmt.Errorf("update components for %s: %w", analyzer.Name, err)
			}
			checks, err := checksBlock(analyzer.Name, analyzer.Checks)
			if err != nil {
				return fmt.Errorf("render checks for %s: %w", analyzer.Name, err)
			}
			contents, err = synchronizeChecks(contents, checks)
			if err != nil {
				return fmt.Errorf("update checks for %s: %w", analyzer.Name, err)
			}
			examples, err := examplesBlock(analyzer.Examples)
			if err != nil {
				return fmt.Errorf("render examples for %s: %w", analyzer.Name, err)
			}
			contents, err = synchronizeExamples(contents, examples)
			if err != nil {
				return fmt.Errorf("update examples for %s: %w", analyzer.Name, err)
			}
			if len(analyzer.Options) > 0 {
				contents, err = synchronizeOptions(contents, optionsTable(analyzer.Options))
				if err != nil {
					return fmt.Errorf("update options for %s: %w", analyzer.Name, err)
				}
			} else if bytes.Contains(contents, []byte(generatedOptionsStart)) || bytes.Contains(contents, []byte("\n## Options\n")) {
				return fmt.Errorf("%s documents options, but analyzer %q has no flags", relativePath(root, page), analyzer.Name)
			}
			updates[page] = contents
		}
	}

	if err := rejectUnknownPages(root, expectedPages); err != nil {
		return err
	}
	if err := collectSharedUpdates(root, data, updates); err != nil {
		return err
	}

	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := updateFile(root, path, updates[path], check); err != nil {
			return err
		}
	}
	return nil
}

// collectSharedUpdates adds the pages that are not tied to one analyzer: the
// site's analyzer manifest, the catalog index, and the generated blocks in the
// development guides.
func collectSharedUpdates(root string, data manifest, updates map[string][]byte) error {
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	updates[filepath.Join(root, "site", "src", "generated", "analyzers.json")] = append(encoded, '\n')
	updates[filepath.Join(root, "docs", "analyzers", "index.md")] = []byte(analyzerIndex(data))
	return synchronizeDevelopmentDocs(root, updates)
}

func rejectUnknownPages(root string, expected map[string]bool) error {
	directory := filepath.Join(root, "docs", "analyzers")
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "index.md" {
			return nil
		}
		extension := filepath.Ext(path)
		if extension != ".md" && extension != ".mdx" {
			return nil
		}
		if !expected[path] {
			return fmt.Errorf("documentation page %s has no registered analyzer", relativePath(root, path))
		}
		return nil
	})
}

func updateFile(root, path string, expected []byte, check bool) error {
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if bytes.Equal(current, expected) {
		return nil
	}
	if check {
		return fmt.Errorf("generated documentation is stale: %s (run go generate ./...)", relativePath(root, path))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, expected, 0o644); err != nil {
		return err
	}
	fmt.Printf("updated %s\n", relativePath(root, path))
	return nil
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
