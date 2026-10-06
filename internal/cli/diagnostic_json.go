package cli

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

// The JSON boundary merges go vet's package documents and resolves diagnostic
// positions before rendering. Malformed output remains distinct from an empty
// diagnostic set, and analyzer errors take precedence when choosing status.

type jsonDiagnostic struct {
	Category string        `json:"category"`
	Posn     string        `json:"posn"`
	End      string        `json:"end"`
	Message  string        `json:"message"`
	Related  []jsonRelated `json:"related"`
}

type jsonRelated struct {
	Posn    string `json:"posn"`
	End     string `json:"end"`
	Message string `json:"message"`
}

type positionedDiagnostic struct {
	Analyzer string
	// Check is the stable check ID, such as resourcelifetime/missing-release.
	Check   string
	Start   sourcePosition
	End     sourcePosition
	Message string
	Related []jsonRelated
}

type sourcePosition struct {
	Filename string
	Line     int
	Column   int
}

func jsonDiagnosticExitCode(data []byte) int {
	diagnostics, analysisErrors, err := decodeDiagnostics(data)
	if err != nil {
		return 1
	}
	return diagnosticExitCode(diagnostics, analysisErrors)
}

// mergeVetOutput folds the JSON objects go vet prints, one per analyzed
// package, into the single document the renderers decode. A package pattern
// that matches several packages, or one package with a test variant, yields
// several objects, so treating stdout as one document would mistake every
// multi-package run for a build failure. Empty output is an empty document.
func mergeVetOutput(data []byte) ([]byte, error) {
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

func diagnosticExitCode(diagnostics []positionedDiagnostic, analysisErrors []string) int {
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
