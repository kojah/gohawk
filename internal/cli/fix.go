package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"

	udiff "github.com/aymanbagabas/go-udiff"
)

// applySuggestedFixes applies (or, with diff, previews) the suggested fixes
// carried in go vet's JSON output. go vet has no fix mode of its own, so gohawk
// reads the edits from the JSON and rewrites the files itself. The byte offsets
// are file offsets, and the edits of the first suggested fix on each diagnostic
// are applied; overlapping edits are dropped so a rewrite never corrupts a file.
func applySuggestedFixes(data []byte, diff bool, output, errorsOutput io.Writer) int {
	edits, err := collectFixEdits(data)
	if err != nil {
		writeFormattedf(errorsOutput, "gohawk: decode analyzer output: %v\n", err)
		return 1
	}
	failed := false
	for _, filename := range slices.Sorted(maps.Keys(edits)) {
		original, err := os.ReadFile(filename)
		if err != nil {
			writeFormattedf(errorsOutput, "gohawk: read %s: %v\n", filename, err)
			failed = true
			continue
		}
		updated, changed := applyFileEdits(original, edits[filename])
		if !changed {
			continue
		}
		if diff {
			_, _ = io.WriteString(output, udiff.Unified(filename, filename, string(original), string(updated)))
			continue
		}
		if err := os.WriteFile(filename, updated, 0o644); err != nil {
			writeFormattedf(errorsOutput, "gohawk: write %s: %v\n", filename, err)
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

type fileEdit struct {
	start, end int
	text       string
}

// collectFixEdits gathers the first suggested fix of every diagnostic, grouped
// by file. go vet reports a package and its test variant separately, so the
// same edit can appear twice; identical edits are deduplicated.
func collectFixEdits(data []byte) (map[string][]fileEdit, error) {
	var tree map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	edits := map[string][]fileEdit{}
	seen := map[string]bool{}
	for _, analyzers := range tree {
		for _, raw := range analyzers {
			if len(raw) == 0 || raw[0] != '[' {
				continue // an {"error": ...} entry carries no diagnostics
			}
			var items []jsonDiagnostic
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, err
			}
			for _, item := range items {
				if len(item.SuggestedFixes) == 0 {
					continue
				}
				for _, edit := range item.SuggestedFixes[0].Edits {
					key := edit.Filename + "\x00" + strconv.Itoa(edit.Start) + "\x00" + strconv.Itoa(edit.End) + "\x00" + edit.New
					if seen[key] {
						continue
					}
					seen[key] = true
					edits[edit.Filename] = append(edits[edit.Filename], fileEdit{start: edit.Start, end: edit.End, text: edit.New})
				}
			}
		}
	}
	return edits, nil
}

// applyFileEdits splices non-overlapping edits into original, left to right.
func applyFileEdits(original []byte, edits []fileEdit) ([]byte, bool) {
	slices.SortFunc(edits, func(a, b fileEdit) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return a.end - b.end
	})
	var buffer bytes.Buffer
	position := 0
	for _, edit := range edits {
		if edit.start < position || edit.start > edit.end || edit.end > len(original) {
			continue // out of range or overlapping a preceding edit
		}
		buffer.Write(original[position:edit.start])
		buffer.WriteString(edit.text)
		position = edit.end
	}
	if position == 0 {
		return nil, false
	}
	buffer.Write(original[position:])
	return buffer.Bytes(), true
}
