// Command gendocs synchronizes analyzer metadata with the documentation site.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/docexamples"
)

const (
	generatedAnalyzersStart  = "<!-- gohawk:generated-analyzers:start -->"
	generatedAnalyzersEnd    = "<!-- gohawk:generated-analyzers:end -->"
	generatedOptionsStart    = "{/* gohawk:generated-options:start */}"
	generatedOptionsEnd      = "{/* gohawk:generated-options:end */}"
	generatedExamplesStart   = "{/* gohawk:generated-examples:start */}"
	generatedExamplesEnd     = "{/* gohawk:generated-examples:end */}"
	generatedChecksStart     = "{/* gohawk:generated-checks:start */}"
	generatedChecksEnd       = "{/* gohawk:generated-checks:end */}"
	analyzerComponentImports = "import CheckIdentity from '../../../site/src/components/CheckIdentity.astro';"
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
	OptIn        bool            `json:"optIn"`
	Checks       []check         `json:"checks"`
	SuggestedFix bool            `json:"suggestedFix"`
	Options      []optionFlag    `json:"options"`
	Examples     docexamples.Set `json:"-"`
}

type check struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	OptIn   bool   `json:"optIn"`
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
	analyzerGroups := gohawk.AnalyzerGroups()
	targets := make([]docexamples.Target, 0, len(metadata))
	for _, analyzerGroup := range analyzerGroups {
		for _, registered := range analyzerGroup.Analyzers {
			if seen[registered.Name] {
				return manifest{}, fmt.Errorf("analyzer %q is registered more than once", registered.Name)
			}
			seen[registered.Name] = true
			if _, ok := metadata[registered.Name]; !ok {
				return manifest{}, fmt.Errorf("analyzer %q has no metadata", registered.Name)
			}
			targets = append(targets, docexamples.Target{
				TestRoot: filepath.Join(root, "internal", "analyzers", analyzerGroup.Name, registered.Name, "testdata"),
				Analyzer: registered,
			})
		}
	}
	for name := range metadata {
		if !seen[name] {
			return manifest{}, fmt.Errorf("metadata exists for unknown analyzer %q", name)
		}
	}
	examples, err := docexamples.CollectAll(targets)
	if err != nil {
		return manifest{}, err
	}

	result := manifest{}
	for _, analyzerGroup := range analyzerGroups {
		group := group{
			Name:  analyzerGroup.Name,
			Title: heading(analyzerGroup.Doc),
			Slug:  analyzerGroup.DocPath,
		}
		for _, registered := range analyzerGroup.Analyzers {
			info := metadata[registered.Name]
			item := analyzer{
				Name:         registered.Name,
				Summary:      sentence(registered.Doc),
				Path:         "analyzers/" + group.Slug + "/" + registered.Name,
				OptIn:        info.OptIn,
				Checks:       checkManifest(info.OptIn, info.Checks),
				SuggestedFix: info.SuggestedFix,
				Options:      []optionFlag{},
				Examples:     examples[registered.Name],
			}
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
	return result, nil
}

func checkManifest(analyzerOptIn bool, checks []gohawk.AnalyzerCheckInfo) []check {
	result := make([]check, len(checks))
	for index, item := range checks {
		result[index] = check{ID: string(item.ID), Summary: item.Doc, OptIn: analyzerOptIn || item.OptIn}
	}
	return result
}

func synchronizeChecks(contents []byte, block string) ([]byte, error) {
	if bytes.Contains(contents, []byte(generatedChecksStart)) {
		return replaceGeneratedBlock(contents, generatedChecksStart, generatedChecksEnd, block)
	}
	if bytes.Contains(contents, []byte("\n### Checks\n")) {
		return nil, errors.New("checks subsection exists without generated block markers")
	}
	const heading = "\n## What it detects\n"
	start := bytes.Index(contents, []byte(heading))
	if start < 0 {
		return nil, errors.New("missing What it detects section")
	}
	bodyStart := start + len(heading)
	end := bytes.Index(contents[bodyStart:], []byte("\n## "))
	if end < 0 {
		return nil, errors.New("what it detects must be followed by another section")
	}
	end += bodyStart
	section := []byte("\n### Checks\n\n" + generatedChecksStart + "\n" + block + "\n" + generatedChecksEnd + "\n")
	result := make([]byte, 0, len(contents)+len(section))
	result = append(result, contents[:end]...)
	result = append(result, section...)
	result = append(result, contents[end:]...)
	return result, nil
}

func synchronizeAnalyzerComponents(contents []byte) ([]byte, error) {
	if bytes.Contains(contents, []byte(analyzerComponentImports)) {
		return contents, nil
	}
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return nil, errors.New("missing frontmatter")
	}
	frontmatterEnd := bytes.Index(contents[len("---\n"):], []byte("\n---\n"))
	if frontmatterEnd < 0 {
		return nil, errors.New("unterminated frontmatter")
	}
	frontmatterEnd += len("---\n") + len("\n---\n")

	result := make([]byte, 0, len(contents)+len(analyzerComponentImports)+2)
	result = append(result, contents[:frontmatterEnd]...)
	result = append(result, '\n')
	result = append(result, analyzerComponentImports...)
	result = append(result, '\n')
	result = append(result, contents[frontmatterEnd:]...)
	return result, nil
}

// checksBlock renders stable check identifiers and summaries as a standard
// Markdown table. The MDX component provides shared opt-in styling while
// preserving Starlight's native table rendering.
func checksBlock(analyzerName string, checks []check) (string, error) {
	var output strings.Builder
	hasOptIn := false
	output.WriteString("| Check | What it detects |\n")
	output.WriteString("| --- | --- |\n")
	for _, item := range checks {
		localID, ok := strings.CutPrefix(item.ID, analyzerName+"/")
		if !ok || localID == "" {
			return "", fmt.Errorf("check ID %q does not use analyzer prefix %q", item.ID, analyzerName+"/")
		}
		optIn := ""
		if item.OptIn {
			optIn = " optIn"
			hasOptIn = true
		}
		fmt.Fprintf(
			&output,
			"| <CheckIdentity name=\"%s\"%s /> | %s |\n",
			html.EscapeString(localID),
			optIn,
			markdownTableCell(item.Summary),
		)
	}
	if hasOptIn {
		output.WriteString("\n\\* Opt-in; requires explicit selection.\n")
	}
	return strings.TrimSuffix(output.String(), "\n"), nil
}

func markdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
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

func examplesBlock(examples docexamples.Set) (string, error) {
	var result strings.Builder
	result.WriteString("### Flagged code\n")
	for index, example := range examples.Flagged {
		result.WriteString("\n")
		if len(examples.Flagged) > 1 {
			title := example.Title
			if title == "" {
				title = fmt.Sprintf("Case %d", index+1)
			}
			fmt.Fprintf(&result, "#### %s\n\n", title)
		}
		metadata, err := json.Marshal(example.Diagnostics)
		if err != nil {
			return "", err
		}
		encoded := base64.RawURLEncoding.EncodeToString(metadata)
		fmt.Fprintf(&result, "```go gohawk=\"%s\"\n%s\n```\n", encoded, example.Code)
	}
	fmt.Fprintf(&result, "\n### Accepted code\n\n```go\n%s\n```", examples.OK.Code)
	return result.String(), nil
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
		return nil, errors.New("options section exists without generated block markers")
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

// groupIntros carries each group's one-sentence introduction, shown under its
// heading on the catalog page. Keyed by group name.
var groupIntros = map[string]string{
	"contracts":   "These analyzers make contracts visible in Go types and APIs, where callers and tools can rely on them.",
	"ownership":   "These analyzers look for work or resources whose owner cannot be identified on every relevant path.",
	"reliability": "These analyzers cover failure modes that often survive ordinary type checking and code review.",
	"testing":     "These analyzers keep test failures bounded and make helper behavior visible at the call site.",
}

func analyzerIndex(data manifest) string {
	var output strings.Builder
	output.WriteString("---\ntitle: All analyzers\ndescription: The gohawk analyzer catalog, generated from the registered Go analyzers.\n---\n\n")
	output.WriteString("<!-- Run go generate ./... to update this page; do not edit it by hand. -->\n\n")
	output.WriteString("gohawk ships a focused set of analyzers rather than a general-purpose lint\n")
	output.WriteString("catalog.\n")
	for _, group := range data.Groups {
		fmt.Fprintf(&output, "\n## %s\n\n", group.Title)
		if intro := groupIntros[group.Name]; intro != "" {
			output.WriteString(intro + "\n\n")
		}
		output.WriteString(groupCards(group))
		output.WriteByte('\n')
	}
	return output.String()
}

// groupCards renders a group's analyzers as a grid of linked cards. The output
// is raw HTML because Markdown tables cannot carry the card layout; anything
// inside it is therefore escaped here rather than by the Markdown renderer.
func groupCards(group group) string {
	var output strings.Builder
	output.WriteString(`<div class="analyzer-grid">` + "\n")
	for _, analyzer := range group.Analyzers {
		link := group.Slug + "/" + analyzer.Name + "/"
		fmt.Fprintf(&output, "  "+`<a class="analyzer-card" href="%s">`+"\n", html.EscapeString(link))
		fmt.Fprintf(&output, "    "+`<span class="analyzer-name">%s</span>`+"\n", html.EscapeString(analyzer.Name))
		fmt.Fprintf(&output, "    "+`<span class="analyzer-detects">%s</span>`+"\n", inlineCode(analyzer.Summary))
		output.WriteString("  </a>\n")
	}
	output.WriteString("</div>")
	return output.String()
}

// inlineCode escapes text for HTML and renders `backtick` spans as code, which
// the Markdown renderer would otherwise leave alone inside a raw HTML block.
func inlineCode(text string) string {
	var output strings.Builder
	for index, segment := range strings.Split(text, "`") {
		if index%2 == 1 {
			output.WriteString("<code>" + html.EscapeString(segment) + "</code>")
			continue
		}
		output.WriteString(html.EscapeString(segment))
	}
	return output.String()
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

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
