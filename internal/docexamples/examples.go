// Package docexamples runs analyzers against the living examples used by the
// documentation site.
package docexamples

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

const (
	flaggedMarker = "//gohawk:example flagged"
	okMarker      = "//gohawk:example ok"
	endMarker     = "//gohawk:example end"
)

var wantComment = regexp.MustCompile(`[ \t]+// want .*$`)

// Set contains the incorrect and correct examples for an analyzer.
type Set struct {
	Flagged []Example
	OK      Example
}

// Example is source displayed in the documentation and the diagnostics that
// apply to it.
type Example struct {
	Title       string
	Code        string
	Diagnostics []Diagnostic
}

// Diagnostic is an analyzer message and its zero-based, end-exclusive source
// range within an example.
type Diagnostic struct {
	Check     string `json:"check"`
	Message   string `json:"message"`
	StartLine int    `json:"startLine"`
	StartCol  int    `json:"startColumn"`
	EndLine   int    `json:"endLine"`
	EndCol    int    `json:"endColumn"`
}

// Target identifies one analyzer and its analysistest GOPATH fixture root.
type Target struct {
	TestRoot string
	Analyzer *analysis.Analyzer
}

type region struct {
	kind     string
	title    string
	filename string
	start    int
	end      int
	source   string
	code     string
}

// Collect loads and analyzes an analyzer's fixture package from an analysistest
// GOPATH root. Diagnostics outside marked documentation regions are ordinary
// regression findings and ignored.
func Collect(testRoot string, analyzer *analysis.Analyzer) (Set, error) {
	sets, err := CollectAll([]Target{{TestRoot: testRoot, Analyzer: analyzer}})
	return sets[analyzer.Name], err
}

// CollectAll loads every fixture package in one go/packages invocation, then
// runs each analyzer only against its own package roots. A shared load avoids
// paying the go-list and type-checking startup cost once per documentation
// page while preserving analyzer isolation.
func CollectAll(targets []Target) (map[string]Set, error) {
	results := make(map[string]Set, len(targets))
	if len(targets) == 0 {
		return results, nil
	}
	type preparedTarget struct {
		Target
		regions []region
	}
	prepared := make([]preparedTarget, 0, len(targets))
	testRoots := make([]string, 0, len(targets))
	patterns := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	hasTests := false
	for _, target := range targets {
		if target.Analyzer == nil {
			return nil, errors.New("nil analyzer target")
		}
		if seen[target.Analyzer.Name] {
			return nil, fmt.Errorf("duplicate analyzer target %q", target.Analyzer.Name)
		}
		seen[target.Analyzer.Name] = true
		directory := filepath.Join(target.TestRoot, "src", target.Analyzer.Name)
		regions, targetHasTests, err := readRegions(directory)
		if err != nil {
			return nil, fmt.Errorf("%s examples: %w", target.Analyzer.Name, err)
		}
		prepared = append(prepared, preparedTarget{Target: target, regions: regions})
		testRoots = append(testRoots, target.TestRoot)
		patterns = append(patterns, target.Analyzer.Name)
		hasTests = hasTests || targetHasTests
	}

	environment := slices.DeleteFunc(os.Environ(), func(value string) bool {
		return strings.HasPrefix(value, "GO111MODULE=") || strings.HasPrefix(value, "GOPATH=")
	})
	environment = append(environment, "GO111MODULE=off", "GOPATH="+strings.Join(testRoots, string(os.PathListSeparator)))
	config := &packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   filepath.Join(targets[0].TestRoot, "src", targets[0].Analyzer.Name),
		Env:   environment,
		Tests: hasTests,
	}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, errors.New("no packages were loaded")
	}
	if count := packages.PrintErrors(loaded); count > 0 {
		return nil, fmt.Errorf("fixture packages have %d load errors", count)
	}
	for _, target := range prepared {
		var roots []*packages.Package
		for _, loadedPackage := range loaded {
			if packageBelongsToAnalyzer(loadedPackage, target.Analyzer.Name) {
				roots = append(roots, loadedPackage)
			}
		}
		if len(roots) == 0 {
			return nil, fmt.Errorf("%s: no package was loaded", target.Analyzer.Name)
		}
		graph, err := checker.Analyze([]*analysis.Analyzer{target.Analyzer}, roots, &checker.Options{Sequential: true})
		if err != nil {
			return nil, err
		}
		result, err := collectDiagnostics(target.Analyzer.Name, target.regions, graph.Roots)
		if err != nil {
			return nil, err
		}
		results[target.Analyzer.Name] = result
	}
	return results, nil
}

func packageBelongsToAnalyzer(loaded *packages.Package, analyzerName string) bool {
	return loaded.PkgPath == analyzerName || loaded.PkgPath == analyzerName+".test" || loaded.ForTest == analyzerName
}

func collectDiagnostics(analyzerName string, regions []region, roots []*checker.Action) (Set, error) {
	attachedDiagnostics := make(map[string][]Diagnostic)
	for _, action := range roots {
		if action.Err != nil {
			return Set{}, action.Err
		}
		for _, diagnostic := range action.Diagnostics {
			if err := attachDiagnostic(action.Package.Fset, regions, attachedDiagnostics, diagnostic); err != nil {
				return Set{}, fmt.Errorf("%s: %w", analyzerName, err)
			}
		}
	}

	var result Set
	for _, item := range regions {
		example := Example{Title: item.title, Code: item.code}
		example.Diagnostics = append(example.Diagnostics, attachedDiagnostics[itemKey(item)]...)
		switch item.kind {
		case "flagged":
			result.Flagged = append(result.Flagged, example)
		case "ok":
			result.OK = example
		}
	}
	for index, example := range result.Flagged {
		if len(example.Diagnostics) == 0 {
			return Set{}, fmt.Errorf("%s: flagged example %d produced no diagnostics", analyzerName, index+1)
		}
	}
	if len(result.OK.Diagnostics) > 0 {
		return Set{}, fmt.Errorf("%s: OK example produced %d diagnostics", analyzerName, len(result.OK.Diagnostics))
	}
	return result, nil
}

func readRegions(directory string) ([]region, bool, error) {
	var result []region
	var hasTests bool
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != directory {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			hasTests = true
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsed, err := parseRegions(path, contents)
		if err != nil {
			return err
		}
		result = append(result, parsed...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	counts := map[string]int{}
	for _, item := range result {
		counts[item.kind]++
	}
	if counts["flagged"] < 1 || counts["ok"] != 1 {
		return nil, false, fmt.Errorf("need at least one flagged and exactly one OK region; found %d and %d", counts["flagged"], counts["ok"])
	}
	return result, hasTests, nil
}

func parseRegions(filename string, contents []byte) ([]region, error) {
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	offset, line := 0, 1
	var active *region
	var result []region
	for scanner.Scan() {
		text := scanner.Text()
		lineBytes := len(scanner.Bytes())
		trimmed := strings.TrimSpace(text)
		kind, title, startsRegion := exampleMarker(trimmed)
		switch {
		case startsRegion:
			if active != nil {
				return nil, fmt.Errorf("%s:%d: nested example marker", filename, line)
			}
			active = &region{kind: kind, title: title, filename: filename, start: offset + lineBytes + 1}
		case trimmed == endMarker:
			if active == nil {
				return nil, fmt.Errorf("%s:%d: unmatched end marker", filename, line)
			}
			active.end = offset
			active.source = string(contents[active.start:active.end])
			active.code = displayCode(contents[active.start:active.end])
			result = append(result, *active)
			active = nil
		}
		offset += lineBytes + 1
		line++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if active != nil {
		return nil, fmt.Errorf("%s: unclosed %s example", filename, active.kind)
	}
	return result, nil
}

func exampleMarker(text string) (kind, title string, ok bool) {
	for _, marker := range []struct {
		prefix string
		kind   string
	}{
		{prefix: flaggedMarker, kind: "flagged"},
		{prefix: okMarker, kind: "ok"},
	} {
		if text == marker.prefix {
			return marker.kind, "", true
		}
		if strings.HasPrefix(text, marker.prefix+" ") {
			return marker.kind, strings.TrimSpace(strings.TrimPrefix(text, marker.prefix)), true
		}
	}
	return "", "", false
}

func displayCode(source []byte) string {
	lines := strings.Split(strings.TrimRight(string(source), "\n"), "\n")
	for index, line := range lines {
		line = strings.TrimRight(wantComment.ReplaceAllString(line, ""), " \t")
		lines[index] = strings.ReplaceAll(line, "\t", "  ")
	}
	return strings.Join(lines, "\n")
}

func attachDiagnostic(files *token.FileSet, regions []region, attached map[string][]Diagnostic, raw analysis.Diagnostic) error {
	if raw.Category == "" {
		return fmt.Errorf("diagnostic %q has no check identity", raw.Message)
	}
	start := files.PositionFor(raw.Pos, true)
	end := files.PositionFor(raw.End, true)
	if !start.IsValid() || !end.IsValid() || raw.End <= raw.Pos {
		return fmt.Errorf("diagnostic %q has no precise source range", raw.Message)
	}
	for _, item := range regions {
		if !sameFile(start.Filename, item.filename) || start.Offset < item.start || end.Offset > item.end {
			continue
		}
		startLine, startColumn := lineColumn(item.source, start.Offset-item.start)
		endLine, endColumn := lineColumn(item.source, end.Offset-item.start)
		attached[itemKey(item)] = append(attached[itemKey(item)], Diagnostic{
			Check:     raw.Category,
			Message:   raw.Message,
			StartLine: startLine,
			StartCol:  startColumn,
			EndLine:   endLine,
			EndCol:    endColumn,
		})
		return nil
	}
	return nil
}

func lineColumn(source string, offset int) (line, column int) {
	if offset > len(source) {
		offset = len(source)
	}
	for index := range offset {
		switch source[index] {
		case '\n':
			line++
			column = 0
		case '\t':
			column += 2
		default:
			column++
		}
	}
	return line, column
}

func sameFile(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && leftAbsolute == rightAbsolute
}

func itemKey(item region) string {
	return fmt.Sprintf("%s:%d:%d", item.filename, item.start, item.end)
}
