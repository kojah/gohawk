// Package vetjson decodes the JSON that go vet prints for an analysis tool.
//
// go vet writes one JSON object per analyzed package. This package merges
// those objects and resolves diagnostic positions, so every consumer of a
// delegated run (the terminal renderer, the JSON passthrough, and the editor
// server) reads findings through one decoder. Malformed output stays distinct
// from an empty diagnostic set.
package vetjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"sort"
	"strconv"
	"strings"
)

type rawDiagnostic struct {
	Category string    `json:"category"`
	Posn     string    `json:"posn"`
	End      string    `json:"end"`
	Message  string    `json:"message"`
	Related  []Related `json:"related"`
}

// Related is one supporting location of a diagnostic, with unresolved
// file:line:column positions as go vet printed them.
type Related struct {
	Posn    string `json:"posn"`
	End     string `json:"end"`
	Message string `json:"message"`
}

// Diagnostic is one finding with its positions resolved.
type Diagnostic struct {
	Analyzer string
	// Check is the stable check ID, such as resourcelifetime/missing-release.
	Check   string
	Start   Position
	End     Position
	Message string
	Related []Related
}

// Position is a resolved source position. Column counts bytes from one.
type Position struct {
	Filename string
	Line     int
	Column   int
}

// Merge folds the JSON objects go vet prints, one per analyzed
// package, into the single document the renderers decode. A package pattern
// that matches several packages, or one package with a test variant, yields
// several objects, so treating stdout as one document would mistake every
// multi-package run for a build failure. Empty output is an empty document.
func Merge(data []byte) ([]byte, error) {
	merged := map[string]json.RawMessage{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var object map[string]json.RawMessage
		if err := decoder.Decode(&object); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		maps.Copy(merged, object)
	}
	return json.MarshalIndent(merged, "", "\t")
}

// Decode returns the findings and analyzer errors in a merged document,
// without duplicates and in file, line, column order.
func Decode(data []byte) ([]Diagnostic, []string, error) {
	var tree map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, nil, err
	}

	var diagnostics []Diagnostic
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
			var items []rawDiagnostic
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, nil, err
			}
			for _, item := range items {
				start, err := ParsePosition(item.Posn)
				if err != nil {
					return nil, nil, err
				}
				end, err := ParsePosition(item.End)
				if err != nil {
					end = start
				}
				key := analyzer + "\x00" + item.Posn + "\x00" + item.End + "\x00" + item.Message
				if seen[key] {
					continue
				}
				seen[key] = true
				diagnostics = append(diagnostics, Diagnostic{
					Analyzer: analyzer,
					Check:    item.Category,
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

// ParsePosition resolves a file:line:column position. The filename may itself
// contain colons, as Windows drive letters do.
func ParsePosition(value string) (Position, error) {
	lastColon := strings.LastIndexByte(value, ':')
	if lastColon < 0 {
		return Position{}, fmt.Errorf("invalid source position %q", value)
	}
	previousColon := strings.LastIndexByte(value[:lastColon], ':')
	if previousColon < 0 {
		return Position{}, fmt.Errorf("invalid source position %q", value)
	}
	line, lineErr := strconv.Atoi(value[previousColon+1 : lastColon])
	column, columnErr := strconv.Atoi(value[lastColon+1:])
	if lineErr != nil || columnErr != nil {
		return Position{}, fmt.Errorf("invalid source position %q", value)
	}
	return Position{Filename: value[:previousColon], Line: line, Column: column}, nil
}
