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
	directory := filepath.Join(testRoot, "src", analyzer.Name)
	regions, hasTests, err := readRegions(directory)
	if err != nil {
		return Set{}, fmt.Errorf("%s examples: %w", analyzer.Name, err)
	}

	environment := slices.DeleteFunc(os.Environ(), func(value string) bool {
		return strings.HasPrefix(value, "GO111MODULE=") || strings.HasPrefix(value, "GOPATH=")
	})
	environment = append(environment, "GO111MODULE=off", "GOPATH="+testRoot)
	config := &packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   directory,
		Env:   environment,
		Tests: hasTests,
	}
	loaded, err := packages.Load(config, analyzer.Name)
	if err != nil {
		return Set{}, err
	}
	if len(loaded) == 0 {
		return Set{}, errors.New("no package was loaded")
	}
	if count := packages.PrintErrors(loaded); count > 0 {
		return Set{}, fmt.Errorf("fixture package has %d load errors", count)
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{analyzer}, loaded, &checker.Options{Sequential: true})
	if err != nil {
		return Set{}, err
	}
	attachedDiagnostics := make(map[string][]Diagnostic)
	for _, action := range graph.Roots {
		if action.Err != nil {
			return Set{}, action.Err
		}
		for _, diagnostic := range action.Diagnostics {
			if err := attachDiagnostic(action.Package.Fset, regions, attachedDiagnostics, diagnostic); err != nil {
				return Set{}, fmt.Errorf("%s: %w", analyzer.Name, err)
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
			return Set{}, fmt.Errorf("%s: flagged example %d produced no diagnostics", analyzer.Name, index+1)
		}
	}
	if len(result.OK.Diagnostics) > 0 {
		return Set{}, fmt.Errorf("%s: OK example produced %d diagnostics", analyzer.Name, len(result.OK.Diagnostics))
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
