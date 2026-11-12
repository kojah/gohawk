package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/kojah/gohawk/internal/reporting/vetjson"
)

// Conversion from decoded go vet findings to protocol diagnostics. It maps
// overlay paths back to real files and byte columns to the UTF-16 columns the
// protocol counts in, and attaches what the catalog knows about the check.
// It never drops, merges, or reorders findings.

// CheckInfo is what the catalog knows about one check.
type CheckInfo struct {
	Help             string
	DocumentationURL string
	Experimental     bool
}

// Catalog looks a check up by its stable ID. An unknown ID yields the zero
// CheckInfo.
type Catalog func(check string) CheckInfo

const diagnosticSource = "gohawk"

type fileDiagnostic struct {
	path       string
	diagnostic diagnostic
}

type converter struct {
	catalog Catalog
	overlay *overlay
	lines   map[string][]string
}

func newConverter(catalog Catalog, overlay *overlay) *converter {
	return &converter{catalog: catalog, overlay: overlay, lines: make(map[string][]string)}
}

func (c *converter) convert(findings []vetjson.Diagnostic) []fileDiagnostic {
	converted := make([]fileDiagnostic, 0, len(findings))
	for _, finding := range findings {
		path := c.overlay.realPath(finding.Start.Filename)
		info := c.catalog(finding.Check)
		severity := severityWarning
		if info.Experimental {
			severity = severityInformation
		}
		result := diagnostic{
			Range:    c.textRange(finding.Start, finding.End),
			Severity: severity,
			Code:     finding.Check,
			Source:   diagnosticSource,
			Message:  finding.Message,
			Data:     &diagnosticData{Analyzer: finding.Analyzer, Help: info.Help, URI: uriFromPath(path)},
		}
		if info.DocumentationURL != "" {
			result.CodeDescription = &codeDescription{Href: info.DocumentationURL}
		}
		for _, related := range finding.Related {
			if converted, ok := c.related(related); ok {
				result.RelatedInformation = append(result.RelatedInformation, converted)
			}
		}
		converted = append(converted, fileDiagnostic{path: path, diagnostic: result})
	}
	return converted
}

func (c *converter) related(related vetjson.Related) (relatedInformation, bool) {
	start, err := vetjson.ParsePosition(related.Posn)
	if err != nil {
		return relatedInformation{}, false
	}
	end, err := vetjson.ParsePosition(related.End)
	if err != nil {
		end = start
	}
	return relatedInformation{
		Location: location{URI: uriFromPath(c.overlay.realPath(start.Filename)), Range: c.textRange(start, end)},
		Message:  related.Message,
	}, true
}

func (c *converter) textRange(start, end vetjson.Position) textRange {
	return textRange{Start: c.position(start), End: c.position(end)}
}

// position converts a one-based line and byte column to a zero-based line and
// UTF-16 column, using the text the analyzer saw.
func (c *converter) position(source vetjson.Position) position {
	line := max(source.Line-1, 0)
	lines := c.fileLines(c.overlay.realPath(source.Filename))
	if line >= len(lines) {
		return position{Line: line, Character: max(source.Column-1, 0)}
	}
	text := lines[line]
	offset := min(max(source.Column-1, 0), len(text))
	return position{Line: line, Character: utf16Length(text[:offset])}
}

func (c *converter) fileLines(path string) []string {
	if lines, ok := c.lines[path]; ok {
		return lines
	}
	text, fromBuffer := c.overlay.text(path)
	if !fromBuffer {
		content, err := os.ReadFile(filepath.Clean(path))
		if err == nil {
			text = string(content)
		}
	}
	lines := strings.Split(text, "\n")
	c.lines[path] = lines
	return lines
}

func utf16Length(text string) int {
	if utf8.ValidString(text) {
		units := 0
		for _, character := range text {
			units += utf16.RuneLen(character)
		}
		return units
	}
	return len(text)
}
