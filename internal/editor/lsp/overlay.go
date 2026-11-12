package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// An overlay lets the go command analyze unsaved buffers: each buffer is
// written to a temporary file and named as the replacement for its real path.
// Diagnostics then point at the temporary file, so the overlay also maps those
// positions back. Line and column are unchanged because the replacement is
// the buffer itself.

type overlay struct {
	directory string
	file      string
	original  map[string]string // temporary path to real path
	texts     map[string]string // real path to the analyzed text
}

// writeOverlay returns nil when there is nothing unsaved to describe.
func writeOverlay(parent string, texts map[string]string) (*overlay, error) {
	if len(texts) == 0 {
		return nil, nil //nolint:nilnil // no overlay is a normal outcome, not an error
	}
	directory, err := os.MkdirTemp(parent, "overlay-")
	if err != nil {
		return nil, err
	}
	result := &overlay{directory: directory, original: make(map[string]string), texts: texts}
	replace := make(map[string]string, len(texts))
	index := 0
	for path, text := range texts {
		temporary := filepath.Join(directory, fmt.Sprintf("%d-%s", index, filepath.Base(path)))
		index++
		if err := os.WriteFile(temporary, []byte(text), 0o600); err != nil {
			result.remove()
			return nil, err
		}
		replace[path] = temporary
		result.original[temporary] = path
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

// realPath maps a reported filename back to the file the user sees.
func (o *overlay) realPath(reported string) string {
	if o == nil {
		return reported
	}
	if real, ok := o.original[filepath.Clean(reported)]; ok {
		return real
	}
	return reported
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
