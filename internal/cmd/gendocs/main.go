// Command gendocs synchronizes analyzer metadata with the documentation site.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/kojah/gohawk"
	"github.com/kojah/gohawk/internal/docexamples"
)

const (
	generatedAnalyzersStart = "<!-- gohawk:generated-analyzers:start -->"
	generatedAnalyzersEnd   = "<!-- gohawk:generated-analyzers:end -->"
	generatedOptionsStart   = "<!-- gohawk:generated-options:start -->"
	generatedOptionsEnd     = "<!-- gohawk:generated-options:end -->"
	generatedExamplesStart  = "<!-- gohawk:generated-examples:start -->"
	generatedExamplesEnd    = "<!-- gohawk:generated-examples:end -->"
)

type manifest struct {
	Groups []group `json:"groups"`
}

type group struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Slug      string     `json:"slug"`
	Analyzers []analyzer `json:"analyzers"`
}

type analyzer struct {
	Name         string          `json:"name"`
	Summary      string          `json:"summary"`
	Path         string          `json:"path"`
	SuggestedFix bool            `json:"suggestedFix"`
	Options      []optionFlag    `json:"options"`
	Examples     docexamples.Set `json:"-"`
}

type optionFlag struct {
	Name    string `json:"name"`
	Default string `json:"default"`
	Usage   string `json:"usage"`
}

func main() {
	check := flag.Bool("check", false, "fail if generated documentation is stale")
	flag.Parse()

	root, err := repositoryRoot()
	if err == nil {
		err = synchronize(root, *check)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("could not find repository root containing go.mod")
		}
		directory = parent
	}
}

func synchronize(root string, check bool) error {
	data, err := collectManifest(root)
	if err != nil {
		return err
	}

	expectedPages := make(map[string]bool)
	updates := make(map[string][]byte)
	for _, group := range data.Groups {
		groupIndex := filepath.Join(root, "docs", "analyzers", group.Slug, "index.md")
		contents, err := os.ReadFile(groupIndex)
		if err != nil {
			return fmt.Errorf("read group page %s: %w", relativePath(root, groupIndex), err)
		}
		contents, err = replaceGeneratedBlock(contents, generatedAnalyzersStart, generatedAnalyzersEnd, groupTable(group, false))
		if err != nil {
			return fmt.Errorf("update %s: %w", relativePath(root, groupIndex), err)
		}
		updates[groupIndex] = contents

		for _, analyzer := range group.Analyzers {
			page := filepath.Join(root, "docs", filepath.FromSlash(analyzer.Path+".md"))
			expectedPages[page] = true
			contents, err := os.ReadFile(page)
			if err != nil {
				return fmt.Errorf("analyzer %q has no documentation page at %s", analyzer.Name, relativePath(root, page))
			}
			if !hasFrontmatterTitle(contents, analyzer.Name) {
				return fmt.Errorf("%s must have frontmatter title %q", relativePath(root, page), analyzer.Name)
			}
			contents, err = synchronizeExamples(contents, examplesBlock(analyzer.Examples))
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

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	updates[filepath.Join(root, "site", "src", "generated", "analyzers.json")] = append(encoded, '\n')
	updates[filepath.Join(root, "docs", "analyzers", "index.md")] = []byte(analyzerIndex(data))

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

func collectManifest(root string) (manifest, error) {
	metadata := gohawk.AnalyzerMetadata()
	seen := make(map[string]bool)
	result := manifest{}
	for _, analyzerGroup := range gohawk.AnalyzerGroups() {
		group := group{
			Name:  analyzerGroup.Name,
			Title: heading(analyzerGroup.Doc),
			Slug:  slugify(analyzerGroup.Doc),
		}
		for _, registered := range analyzerGroup.Analyzers {
			if seen[registered.Name] {
				return manifest{}, fmt.Errorf("analyzer %q is registered more than once", registered.Name)
			}
			seen[registered.Name] = true
			info, ok := metadata[registered.Name]
			if !ok {
				return manifest{}, fmt.Errorf("analyzer %q has no metadata", registered.Name)
			}
			item := analyzer{
				Name:         registered.Name,
				Summary:      sentence(registered.Doc),
				Path:         "analyzers/" + group.Slug + "/" + registered.Name,
				SuggestedFix: info.SuggestedFix,
				Options:      []optionFlag{},
			}
			examples, err := docexamples.Collect(root, registered)
			if err != nil {
				return manifest{}, err
			}
			item.Examples = examples
			registered.Flags.VisitAll(func(value *flag.Flag) {
				item.Options = append(item.Options, optionFlag{
					Name:    value.Name,
					Default: value.DefValue,
					Usage:   sentence(value.Usage),
				})
			})
			group.Analyzers = append(group.Analyzers, item)
		}
		result.Groups = append(result.Groups, group)
	}
	for name := range metadata {
		if !seen[name] {
			return manifest{}, fmt.Errorf("metadata exists for unknown analyzer %q", name)
		}
	}
	return result, nil
}

func synchronizeExamples(contents []byte, examples string) ([]byte, error) {
	if bytes.Contains(contents, []byte(generatedExamplesStart)) {
		updated, err := replaceGeneratedBlock(contents, generatedExamplesStart, generatedExamplesEnd, examples)
		if err != nil {
			return nil, err
		}
		doubleBlank := []byte("\n## Examples\n\n\n" + generatedExamplesStart)
		singleBlank := []byte("\n## Examples\n\n" + generatedExamplesStart)
		return bytes.Replace(updated, doubleBlank, singleBlank, 1), nil
	}
	const heading = "\n## Examples\n"
	start := bytes.Index(contents, []byte(heading))
	if start < 0 {
		return nil, errors.New("missing Examples section")
	}
	bodyStart := start + len(heading)
	end := bytes.Index(contents[bodyStart:], []byte("\n## "))
	if end < 0 {
		end = len(contents)
	} else {
		end += bodyStart
	}
	replacement := []byte("\n" + generatedExamplesStart + "\n" + examples + "\n" + generatedExamplesEnd + "\n")
	result := make([]byte, 0, len(contents)-(end-bodyStart)+len(replacement))
	result = append(result, contents[:bodyStart]...)
	result = append(result, replacement...)
	result = append(result, contents[end:]...)
	return result, nil
}

func examplesBlock(examples docexamples.Set) string {
	metadata, err := json.Marshal(examples.Flagged.Diagnostics)
	if err != nil {
		panic(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(metadata)
	return fmt.Sprintf("### Flagged\n\n```go gohawk=\"%s\"\n%s\n```\n\n### OK\n\n```go\n%s\n```", encoded, examples.Flagged.Code, examples.OK.Code)
}

func rejectUnknownPages(root string, expected map[string]bool) error {
	directory := filepath.Join(root, "docs", "analyzers")
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" || entry.Name() == "index.md" {
			return nil
		}
		if !expected[path] {
			return fmt.Errorf("documentation page %s has no registered analyzer", relativePath(root, path))
		}
		return nil
	})
}

func replaceGeneratedBlock(contents []byte, start, end, body string) ([]byte, error) {
	startIndex := bytes.Index(contents, []byte(start))
	endIndex := bytes.Index(contents, []byte(end))
	if startIndex < 0 || endIndex < 0 || endIndex < startIndex {
		return nil, fmt.Errorf("missing generated block markers %q and %q", start, end)
	}
	endIndex += len(end)
	replacement := []byte(start + "\n" + body + "\n" + end)
	result := make([]byte, 0, len(contents)-endIndex+startIndex+len(replacement))
	result = append(result, contents[:startIndex]...)
	result = append(result, replacement...)
	result = append(result, contents[endIndex:]...)
	return result, nil
}

func synchronizeOptions(contents []byte, table string) ([]byte, error) {
	table = strings.TrimSpace(table)
	if bytes.Contains(contents, []byte(generatedOptionsStart)) {
		return replaceGeneratedBlock(contents, generatedOptionsStart, generatedOptionsEnd, table)
	}
	if bytes.Contains(contents, []byte("\n## Options\n")) {
		return nil, errors.New("Options section exists without generated block markers")
	}
	contents = bytes.TrimRight(contents, "\n")
	contents = append(contents, []byte("\n\n## Options\n\n"+generatedOptionsStart+"\n"+table+"\n"+generatedOptionsEnd+"\n")...)
	return contents, nil
}

func hasFrontmatterTitle(contents []byte, title string) bool {
	lines := strings.Split(string(contents), "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return false
	}
	for _, line := range lines[1:] {
		if line == "---" {
			break
		}
		if strings.TrimSpace(line) == "title: "+title {
			return true
		}
	}
	return false
}

func analyzerIndex(data manifest) string {
	var output strings.Builder
	output.WriteString("---\ntitle: Analyzers\ndescription: The gohawk analyzer catalog, generated from the registered Go analyzers.\n---\n\n")
	output.WriteString("<!-- Run go generate ./... to update this page; do not edit it by hand. -->\n\n")
	output.WriteString("gohawk ships a focused set of analyzers rather than a general-purpose lint\n")
	output.WriteString("catalog. Every analyzer is enabled by default; select individual analyzers when\n")
	output.WriteString("you want a narrower run.\n")
	for _, group := range data.Groups {
		fmt.Fprintf(&output, "\n## %s\n\n", group.Title)
		output.WriteString(groupTable(group, true))
		output.WriteByte('\n')
	}
	return output.String()
}

func groupTable(group group, includeGroup bool) string {
	var output strings.Builder
	output.WriteString("| Analyzer | What it catches |\n| --- | --- |\n")
	for _, analyzer := range group.Analyzers {
		link := analyzer.Name + "/"
		if includeGroup {
			link = group.Slug + "/" + link
		}
		fmt.Fprintf(&output, "| [`%s`](%s) | %s |\n", analyzer.Name, link, analyzer.Summary)
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func optionsTable(options []optionFlag) string {
	var output strings.Builder
	output.WriteString("| Knob | Default | Effect |\n| --- | --- | --- |\n")
	for _, option := range options {
		defaultValue := option.Default
		if defaultValue == "" {
			defaultValue = "empty"
		} else {
			defaultValue = "`" + strings.ReplaceAll(defaultValue, "`", "\\`") + "`"
		}
		fmt.Fprintf(&output, "| `%s` | %s | %s |\n", option.Name, defaultValue, option.Usage)
	}
	return strings.TrimSuffix(output.String(), "\n")
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

func sentence(value string) string {
	value = heading(value)
	if value == "" {
		return value
	}
	if !strings.HasSuffix(value, ".") {
		value += "."
	}
	return value
}

func heading(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func slugify(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), " ", "-")
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
