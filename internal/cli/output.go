package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const richOutputChild = "GOHAWK_RICH_OUTPUT_CHILD"

type jsonDiagnostic struct {
	Posn    string        `json:"posn"`
	End     string        `json:"end"`
	Message string        `json:"message"`
	Related []jsonRelated `json:"related"`
}

type jsonRelated struct {
	Posn    string `json:"posn"`
	Message string `json:"message"`
}

type positionedDiagnostic struct {
	Analyzer string
	Start    sourcePosition
	End      sourcePosition
	Message  string
	Related  []jsonRelated
}

type sourcePosition struct {
	Filename string
	Line     int
	Column   int
}

type processOutput struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type processExecutor func(name string, arguments, environment []string) (processOutput, error)

func executeProcess(name string, arguments, environment []string) (processOutput, error) {
	command := exec.CommandContext(context.Background(), name, arguments...)
	command.Env = append(os.Environ(), environment...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := processOutput{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		result.exitCode = exitError.ExitCode()
		return result, err
	}
	result.exitCode = -1
	return result, err
}

func useRichOutput(arguments []string, richOutputChildProcess bool) bool {
	if richOutputChildProcess || len(arguments) < 2 {
		return false
	}
	for _, argument := range arguments[1:] {
		name := strings.TrimLeft(argument, "-")
		name, _, _ = strings.Cut(name, "=")
		switch name {
		case "json", "fix", "diff", "flags", "V", "h", "help":
			return false
		}
		if strings.HasSuffix(argument, ".cfg") {
			return false // invocation by go vet -vettool
		}
	}
	return arguments[1] != "help" && arguments[1] != "list"
}

func runWithRichOutput(arguments []string, output io.Writer) int {
	return runWithRichOutputUsing(arguments, output, executeProcess)
}

func runWithRichOutputUsing(arguments []string, output io.Writer, execute processExecutor) int {
	childArguments := make([]string, 0, len(arguments))
	childArguments = append(childArguments, "-json")
	childArguments = append(childArguments, arguments[1:]...)

	result, err := execute(arguments[0], childArguments, []string{richOutputChild + "=1"})
	_, _ = output.Write(result.stderr)
	if err != nil {
		_, _ = output.Write(result.stdout)
		if result.exitCode >= 0 {
			return result.exitCode
		}
		writeFormattedf(output, "gohawk: run analyzer engine: %v\n", err)
		return 1
	}

	diagnostics, analysisErrors, err := decodeDiagnostics(result.stdout)
	if err != nil {
		writeFormattedf(output, "gohawk: decode analyzer output: %v\n", err)
		_, _ = output.Write(result.stdout)
		return 1
	}
	for _, analysisError := range analysisErrors {
		writeLine(output, analysisError)
	}
	contextLines := requestedContext(arguments)
	colors := terminalColors(output)
	for index, diagnostic := range diagnostics {
		if index > 0 {
			writeLine(output)
		}
		renderDiagnostic(output, diagnostic, contextLines, colors)
	}
	if len(analysisErrors) > 0 {
		return 1
	}
	if len(diagnostics) > 0 {
		return 3
	}
	return 0
}

func decodeDiagnostics(data []byte) ([]positionedDiagnostic, []string, error) {
	var tree map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, nil, err
	}

	var diagnostics []positionedDiagnostic
	var analysisErrors []string
	seen := make(map[string]bool)
	for _, analyzers := range tree {
		for analyzer, raw := range analyzers {
			var result struct {
				Error string `json:"error"`
			}
			if len(raw) > 0 && raw[0] == '{' {
				if err := json.Unmarshal(raw, &result); err != nil {
					return nil, nil, err
				}
				if result.Error != "" {
					analysisErrors = append(analysisErrors, analyzer+": "+result.Error)
				}
				continue
			}
			var items []jsonDiagnostic
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, nil, err
			}
			for _, item := range items {
				start, err := parsePosition(item.Posn)
				if err != nil {
					return nil, nil, err
				}
				end, err := parsePosition(item.End)
				if err != nil {
					end = start
				}
				key := analyzer + "\x00" + item.Posn + "\x00" + item.End + "\x00" + item.Message
				if seen[key] {
					continue
				}
				seen[key] = true
				diagnostics = append(diagnostics, positionedDiagnostic{
					Analyzer: analyzer,
					Start:    start,
					End:      end,
					Message:  item.Message,
					Related:  item.Related,
				})
			}
		}
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i], diagnostics[j]
		if a.Start.Filename != b.Start.Filename {
			return a.Start.Filename < b.Start.Filename
		}
		if a.Start.Line != b.Start.Line {
			return a.Start.Line < b.Start.Line
		}
		if a.Start.Column != b.Start.Column {
			return a.Start.Column < b.Start.Column
		}
		return a.Analyzer < b.Analyzer
	})
	sort.Strings(analysisErrors)
	return diagnostics, analysisErrors, nil
}

func parsePosition(value string) (sourcePosition, error) {
	lastColon := strings.LastIndexByte(value, ':')
	if lastColon < 0 {
		return sourcePosition{}, fmt.Errorf("invalid source position %q", value)
	}
	previousColon := strings.LastIndexByte(value[:lastColon], ':')
	if previousColon < 0 {
		return sourcePosition{}, fmt.Errorf("invalid source position %q", value)
	}
	line, lineErr := strconv.Atoi(value[previousColon+1 : lastColon])
	column, columnErr := strconv.Atoi(value[lastColon+1:])
	if lineErr != nil || columnErr != nil {
		return sourcePosition{}, fmt.Errorf("invalid source position %q", value)
	}
	return sourcePosition{Filename: value[:previousColon], Line: line, Column: column}, nil
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
	bold, yellow, cyan, red, reset string
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
		yellow: "\x1b[33m",
		cyan:   "\x1b[36m",
		red:    "\x1b[31m",
		reset:  "\x1b[0m",
	}
}

func renderDiagnostic(output io.Writer, diagnostic positionedDiagnostic, contextLines int, colors colorPalette) {
	writeFormattedf(output, "%s%swarning%s[%s%s%s]: %s%s%s\n",
		colors.bold, colors.yellow, colors.reset,
		colors.bold, diagnostic.Analyzer, colors.reset,
		colors.bold, diagnostic.Message, colors.reset)
	writeFormattedf(output, "  %s-->%s %s:%d:%d\n", colors.cyan, colors.reset,
		diagnostic.Start.Filename, diagnostic.Start.Line, diagnostic.Start.Column)

	if contextLines >= 0 {
		renderSource(output, diagnostic.Start, diagnostic.End, contextLines, colors)
	}
	for _, related := range diagnostic.Related {
		writeFormattedf(output, "  = note: %s: %s\n", related.Posn, related.Message)
	}
}

func renderSource(output io.Writer, start, end sourcePosition, contextLines int, colors colorPalette) {
	data, err := os.ReadFile(start.Filename)
	if err != nil {
		return
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if start.Line < 1 || start.Line > len(lines) {
		return
	}
	if end.Filename != start.Filename || end.Line < start.Line {
		end = start
	}
	if end.Line > len(lines) {
		end.Line = len(lines)
	}
	first := max(1, start.Line-contextLines)
	last := min(len(lines), end.Line+contextLines)
	width := len(strconv.Itoa(last))
	writeFormattedf(output, "%*s %s|%s\n", width, "", colors.cyan, colors.reset)
	for lineNumber := first; lineNumber <= last; lineNumber++ {
		line := lines[lineNumber-1]
		writeFormattedf(output, "%s%*d |%s %s\n", colors.cyan, width, lineNumber, colors.reset, line)
		if lineNumber < start.Line || lineNumber > end.Line {
			continue
		}
		column, length := markerRange(line, lineNumber, start, end)
		writeFormattedf(output, "%*s %s|%s %s%s%s%s\n", width, "", colors.cyan, colors.reset,
			markerIndent(line, column), colors.red, "^"+strings.Repeat("~", length-1), colors.reset)
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
