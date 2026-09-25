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
	"time"
)

func synchronize(root string, check, includeExamples, showTimings bool) error {
	var timings *docsTimings
	if showTimings {
		timings = &docsTimings{started: time.Now(), examples: includeExamples}
		defer func() { _, _ = os.Stderr.WriteString(timings.String()) }()
	}
	started := time.Now()
	data, err := collectManifest(root, includeExamples, timings.exampleMetrics())
	if timings != nil {
		timings.manifest = time.Since(started)
	}
	if err != nil {
		return err
	}

	started = time.Now()
	expectedPages := make(map[string]bool)
	updates := make(map[string][]byte)
	for _, group := range data.Groups {
		for _, analyzer := range group.Analyzers {
			page := filepath.Join(root, "docs", filepath.FromSlash(analyzer.Path+".mdx"))
			expectedPages[page] = true
			contents, err := analyzerPage(root, page, analyzer, includeExamples)
			if err != nil {
				return err
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
	if timings != nil {
		timings.render = time.Since(started)
		timings.analyzers = len(expectedPages)
		timings.pages = len(updates)
	}

	started = time.Now()
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
	if timings != nil {
		timings.write = time.Since(started)
	}
	return nil
}

func analyzerPage(root, page string, analyzer analyzer, includeExamples bool) ([]byte, error) {
	contents, err := os.ReadFile(page)
	if err != nil {
		return nil, fmt.Errorf("analyzer %q has no documentation page at %s", analyzer.Name, relativePath(root, page))
	}
	if err := validateAnalyzerFrontmatter(contents, analyzer.Name); err != nil {
		return nil, fmt.Errorf("%s: %w", relativePath(root, page), err)
	}
	contents, err = synchronizeAnalyzerComponents(contents)
	if err != nil {
		return nil, fmt.Errorf("update components for %s: %w", analyzer.Name, err)
	}
	checks, err := checksBlock(analyzer.Name, analyzer.Checks)
	if err != nil {
		return nil, fmt.Errorf("render checks for %s: %w", analyzer.Name, err)
	}
	contents, err = synchronizeChecks(contents, checks)
	if err != nil {
		return nil, fmt.Errorf("update checks for %s: %w", analyzer.Name, err)
	}
	if includeExamples {
		examples, err := examplesBlock(analyzer.Examples)
		if err != nil {
			return nil, fmt.Errorf("render examples for %s: %w", analyzer.Name, err)
		}
		contents, err = synchronizeExamples(contents, examples)
		if err != nil {
			return nil, fmt.Errorf("update examples for %s: %w", analyzer.Name, err)
		}
	}
	// Analyzers have no options: each check has one mode of operation.
	if bytes.Contains(contents, []byte("\n## Options\n")) {
		return nil, fmt.Errorf("%s documents options, but analyzers have none", relativePath(root, page))
	}
	return contents, nil
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
		return fmt.Errorf("generated documentation is stale: %s (run make generate, or make generate-examples for example changes)", relativePath(root, path))
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
