package lsp

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// An overlay lets the go command analyze unsaved buffers: each buffer is
// written to a temporary file and named as the replacement for its real path.
// Diagnostics then name the temporary file, so its path must lead back to the
// real one. Line and column are unchanged because the replacement is the
// buffer itself.
//
// The real path is encoded into the temporary path, not kept in a table. go
// vet caches analysis output by content, so a later run over the same text
// replays output that names a temporary file from an earlier run, or from an
// earlier server. A per-run table cannot map those; a self-describing path
// can. The replayed positions are still right: a cache hit means the package
// has the same file names and the same contents.

// overlayMarker is the directory component that announces an encoded path.
const overlayMarker = "gohawk-overlay-v1"

// overlayChunk keeps each encoded component below common file name limits.
const overlayChunk = 120

var overlayEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

type overlay struct {
	directory string
	file      string
	texts     map[string]string // real path to the analyzed text
}

// writeOverlay returns nil when there is nothing unsaved to describe.
func writeOverlay(ctx context.Context, parent string, texts map[string]string) (*overlay, error) {
	if len(texts) == 0 {
		return nil, nil //nolint:nilnil // no overlay is a normal outcome, not an error
	}
	directory, err := os.MkdirTemp(parent, "run-")
	if err != nil {
		return nil, err
	}
	result := &overlay{directory: directory, texts: texts}
	replace := make(map[string]string, len(texts))
	for path, text := range texts {
		// A run cancelled by a newer edit must stop writing at once: under
		// an edit storm many runs are started and abandoned.
		if err := ctx.Err(); err != nil {
			result.remove()
			return nil, err
		}
		temporary := encodeOverlayPath(directory, path)
		if err := os.MkdirAll(filepath.Dir(temporary), 0o700); err != nil {
			result.remove()
			return nil, err
		}
		if err := os.WriteFile(temporary, []byte(text), 0o600); err != nil {
			result.remove()
			return nil, err
		}
		replace[path] = temporary
	}
	encoded, err := json.Marshal(map[string]map[string]string{"Replace": replace})
	if err != nil {
		result.remove()
		return nil, err
	}
	result.file = filepath.Join(directory, "overlay.json")
	if err := os.WriteFile(result.file, encoded, 0o600); err != nil {
		result.remove()
		return nil, err
	}
	return result, nil
}

// encodeOverlayPath places the replacement for real under directory, keeping
// the base name so messages that mention the file still read naturally.
func encodeOverlayPath(directory, real string) string {
	encoded := strings.ToLower(overlayEncoding.EncodeToString([]byte(real)))
	components := []string{directory, overlayMarker}
	for len(encoded) > overlayChunk {
		components = append(components, encoded[:overlayChunk])
		encoded = encoded[overlayChunk:]
	}
	components = append(components, encoded, filepath.Base(real))
	return filepath.Join(components...)
}

// decodeOverlayPath recovers the real path from a replacement path written by
// any run of any server. The second result is false for an ordinary path.
func decodeOverlayPath(reported string) (string, bool) {
	components := strings.Split(filepath.ToSlash(filepath.Clean(reported)), "/")
	for index, component := range components {
		if component != overlayMarker || index+2 >= len(components) {
			continue
		}
		encoded := strings.ToUpper(strings.Join(components[index+1:len(components)-1], ""))
		decoded, err := overlayEncoding.DecodeString(encoded)
		if err != nil || filepath.Base(string(decoded)) != components[len(components)-1] {
			return "", false
		}
		return string(decoded), true
	}
	return "", false
}

// text returns the analyzed content of a real path when it came from a
// buffer, so column conversion uses the same bytes the analyzer saw.
func (o *overlay) text(path string) (string, bool) {
	if o == nil {
		return "", false
	}
	text, ok := o.texts[path]
	return text, ok
}

func (o *overlay) remove() {
	if o != nil {
		_ = os.RemoveAll(o.directory)
	}
}
