package cli

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	gohawk "github.com/kojah/gohawk/analyzers"
)

// Terminal rendering formats normalized diagnostics, source spans and related
// evidence. Unavailable source context falls back to location notes; formatting
// never decides which analyzer findings are valid.

func renderDelegatedDiagnostics(data []byte, contextLines int, output io.Writer) int {
	diagnostics, analysisErrors, err := decodeDiagnostics(data)
	if err != nil {
		writeFormattedf(output, "gohawk: decode analyzer output: %v\n", err)
		_, _ = output.Write(data)
		return 1
	}
	for _, analysisError := range analysisErrors {
		writeLine(output, analysisError)
	}
	colors := terminalColors(output)
	for index, diagnostic := range diagnostics {
		if index > 0 {
			writeLine(output)
		}
		renderDiagnostic(output, diagnostic, contextLines, colors)
	}
	renderDocumentationFooter(output, diagnostics)
	return diagnosticExitCode(diagnostics, analysisErrors)
}

// renderDocumentationFooter links each analyzer that reported, once, after
// every diagnostic. A link under each warning would repeat the same page for
// every finding of a run.
func renderDocumentationFooter(output io.Writer, diagnostics []positionedDiagnostic) {
	pages := make(map[string]string)
	for _, group := range gohawk.AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			pages[analyzer.Name] = gohawk.AnalyzerDocumentationURL(group, analyzer.Name)
		}
	}
	var links []string
	listed := make(map[string]bool)
	for _, diagnostic := range diagnostics {
		page, known := pages[diagnostic.Analyzer]
		if !known || listed[diagnostic.Analyzer] {
			continue
		}
		listed[diagnostic.Analyzer] = true
		links = append(links, fmt.Sprintf("  %s: %s", diagnostic.Analyzer, page))
	}
	if len(links) == 0 {
		return
	}
	writeLine(output)
	writeLine(output, "Learn more about these findings:")
	for _, link := range links {
		writeLine(output, link)
	}
	writeLine(output, "To see the full reasoning behind a finding, rerun with")
	writeLine(output, "  -gohawk-trace=<analyzer> -gohawk-trace-candidate=<file:line>")
}

func requestedContext(arguments []string) int {
	contextLines := 0
	for index, argument := range arguments[1:] {
		if after, ok := strings.CutPrefix(argument, "-c="); ok {
			if value, err := strconv.Atoi(after); err == nil {
				contextLines = value
			}
		} else if argument == "-c" && index+2 < len(arguments) {
			if value, err := strconv.Atoi(arguments[index+2]); err == nil {
				contextLines = value
			}
		}
	}
	return contextLines
}

type colorPalette struct {
	bold, dim, yellow, cyan, red, reset string
}

func terminalColors(output io.Writer) colorPalette {
	file, ok := output.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" {
		return colorPalette{}
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return colorPalette{}
	}
	return colorPalette{
		bold:   "\x1b[1m",
		dim:    "\x1b[2m",
		yellow: "\x1b[33m",
		cyan:   "\x1b[36m",
		red:    "\x1b[31m",
		reset:  "\x1b[0m",
	}
}

// renderDiagnostic prints the message first and the stable check ID at the
// end of the line, then the primary span. Evidence in the same file joins the
// primary span in one snippet, in line order with "..." over the gaps, the way
// rustc shows secondary spans; evidence elsewhere gets its own snippet.
func renderDiagnostic(output io.Writer, diagnostic positionedDiagnostic, contextLines int, colors colorPalette) {
	check := diagnostic.Check
	if check == "" {
		check = diagnostic.Analyzer
	}
	writeFormattedf(output, "%s%swarning%s: %s%s%s %s[%s]%s\n",
		colors.bold, colors.yellow, colors.reset,
		colors.bold, diagnostic.Message, colors.reset,
		colors.dim, check, colors.reset)
	writeFormattedf(output, "  %s-->%s %s:%d:%d\n", colors.cyan, colors.reset,
		diagnostic.Start.Filename, diagnostic.Start.Line, diagnostic.Start.Column)
	spans := []labeledSpan{{start: diagnostic.Start, end: diagnostic.End}}
	var elsewhere []jsonRelated
	for _, related := range diagnostic.Related {
		start, err := parsePosition(related.Posn)
		if err != nil || start.Filename != diagnostic.Start.Filename {
			elsewhere = append(elsewhere, related)
			continue
		}
		end, err := parsePosition(related.End)
		if err != nil {
			end = start
		}
		// Evidence that covers exactly the reported code labels the primary
		// marker instead of drawing a second marker under it.
		if start == spans[0].start && end == spans[0].end && spans[0].label == "" {
			spans[0].label = related.Message
			continue
		}
		spans = append(spans, labeledSpan{start: start, end: end, label: related.Message})
	}
	if contextLines < 0 || !renderSnippet(output, spans, contextLines, colors) {
		for _, span := range spans[1:] {
			writeFormattedf(output, "  = note: %s:%d:%d: %s\n", span.start.Filename, span.start.Line, span.start.Column, span.label)
		}
	}
	for _, related := range elsewhere {
		renderRelated(output, related, contextLines, colors)
	}
	if help := checkHelp()[diagnostic.Check]; help != "" {
		writeFormattedf(output, "  %s=%s %shelp:%s %s\n", colors.cyan, colors.reset, colors.bold, colors.reset, help)
	}
}

// checkHelp maps each check ID to its one-sentence fix, from the catalog.
var checkHelp = sync.OnceValue(func() map[string]string {
	help := make(map[string]string)
	for _, info := range gohawk.AnalyzerMetadata() {
		for _, check := range info.Checks {
			help[string(check.ID)] = check.Help
		}
	}
	return help
})

// labeledSpan is a source range with an optional label drawn after its marker.
type labeledSpan struct {
	start, end sourcePosition
	label      string
}

// renderSnippet draws spans from one file in line order, sharing one gutter,
// with "..." where lines between them are skipped. It reports whether the
// source could be read.
func renderSnippet(output io.Writer, spans []labeledSpan, contextLines int, colors colorPalette) bool {
	data, err := os.ReadFile(spans[0].start.Filename)
	if err != nil {
		return false
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	valid := spans[:0:0]
	for _, span := range spans {
		if span.start.Line < 1 || span.start.Line > len(lines) {
			continue
		}
		if span.end.Filename != span.start.Filename || span.end.Line < span.start.Line {
			span.end = span.start
		}
		// A span over several lines, such as a whole go statement, is marked
		// on its first line only; underlining every line of a block buries
		// the evidence around it.
		if span.end.Line > span.start.Line {
			span.end = sourcePosition{Filename: span.start.Filename, Line: span.start.Line, Column: len(lines[span.start.Line-1]) + 1}
		}
		valid = append(valid, span)
	}
	if len(valid) == 0 {
		return false
	}
	sort.SliceStable(valid, func(i, j int) bool { return valid[i].start.Line < valid[j].start.Line })
	// Print each source line once, then the marker of every span covering it,
	// so overlapping spans never hide one another.
	shown := make(map[int]bool)
	for _, span := range valid {
		for lineNumber := max(1, span.start.Line-contextLines); lineNumber <= min(len(lines), span.end.Line+contextLines); lineNumber++ {
			shown[lineNumber] = true
		}
	}
	numbers := slices.Sorted(maps.Keys(shown))
	width := len(strconv.Itoa(numbers[len(numbers)-1]))
	writeFormattedf(output, "%*s %s|%s\n", width, "", colors.cyan, colors.reset)
	for index, lineNumber := range numbers {
		if index > 0 && lineNumber > numbers[index-1]+1 {
			writeFormattedf(output, "%s...%s\n", colors.cyan, colors.reset)
		}
		line := lines[lineNumber-1]
		writeFormattedf(output, "%s%*d |%s %s\n", colors.cyan, width, lineNumber, colors.reset, line)
		for _, span := range valid {
			if lineNumber < span.start.Line || lineNumber > span.end.Line {
				continue
			}
			column, length := markerRange(line, lineNumber, span.start, span.end)
			suffix := ""
			if span.label != "" && lineNumber == span.end.Line {
				suffix = " " + colors.bold + span.label + colors.reset
			}
			writeFormattedf(output, "%*s %s|%s %s%s%s%s%s\n", width, "", colors.cyan, colors.reset,
				markerIndent(line, column), colors.red, "^"+strings.Repeat("~", length-1), colors.reset, suffix)
		}
	}
	return true
}

// renderRelated draws evidence from another file as its own snippet, falling
// back to a note when the source cannot be read.
func renderRelated(output io.Writer, related jsonRelated, contextLines int, colors colorPalette) {
	start, err := parsePosition(related.Posn)
	if err != nil || contextLines < 0 {
		writeFormattedf(output, "  = note: %s: %s\n", related.Posn, related.Message)
		return
	}
	end, err := parsePosition(related.End)
	if err != nil {
		end = start
	}
	writeFormattedf(output, "  %s-->%s %s:%d:%d\n", colors.cyan, colors.reset, start.Filename, start.Line, start.Column)
	if !renderSnippet(output, []labeledSpan{{start: start, end: end, label: related.Message}}, 0, colors) {
		writeFormattedf(output, "  = note: %s\n", related.Message)
	}
}

func markerRange(line string, lineNumber int, start, end sourcePosition) (int, int) {
	column := 1
	if lineNumber == start.Line {
		column = max(1, start.Column)
	}
	lineEnd := len(line) + 1
	markerEnd := lineEnd
	if lineNumber == end.Line {
		markerEnd = max(column+1, end.Column)
	}
	markerEnd = min(markerEnd, lineEnd)
	return column, max(1, markerEnd-column)
}

func markerIndent(line string, column int) string {
	column = min(max(1, column), len(line)+1)
	prefix := line[:column-1]
	var indent strings.Builder
	for _, character := range prefix {
		if character == '\t' {
			indent.WriteByte('\t')
		} else {
			indent.WriteByte(' ')
		}
	}
	return indent.String()
}
