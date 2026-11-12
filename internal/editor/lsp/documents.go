package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Open buffers as the editor last described them. A buffer is unsaved when it
// changed after it was opened or saved; only unsaved buffers need an overlay,
// because every other file is read from disk by the go command.

type document struct {
	path    string
	text    string
	version int
	unsaved bool
}

type documentStore struct {
	byPath map[string]*document
}

func newDocumentStore() *documentStore {
	return &documentStore{byPath: make(map[string]*document)}
}

// open records a buffer the editor opened. An editor can open a file with
// text that differs from the file, as when it restores unsaved work, so the
// buffer is compared with the disk instead of assumed equal to it.
func (s *documentStore) open(path, text string, version int) {
	onDisk, err := os.ReadFile(filepath.Clean(path))
	unsaved := err != nil || string(onDisk) != text
	s.byPath[path] = &document{path: path, text: text, version: version, unsaved: unsaved}
}

func (s *documentStore) change(path, text string, version int) bool {
	current, ok := s.byPath[path]
	if !ok {
		return false
	}
	current.text, current.version, current.unsaved = text, version, true
	return true
}

func (s *documentStore) saved(path string) {
	if current, ok := s.byPath[path]; ok {
		current.unsaved = false
	}
}

// close forgets a buffer and reports whether it held unsaved text, in which
// case the findings on screen described text that no longer exists.
func (s *documentStore) close(path string) (hadUnsavedText bool) {
	current, ok := s.byPath[path]
	delete(s.byPath, path)
	return ok && current.unsaved
}

func (s *documentStore) isOpen(path string) bool {
	_, ok := s.byPath[path]
	return ok
}

// unsavedTexts returns the text of every unsaved buffer by file path.
func (s *documentStore) unsavedTexts() map[string]string {
	texts := make(map[string]string)
	for path, current := range s.byPath {
		if current.unsaved {
			texts[path] = current.text
		}
	}
	return texts
}

// pathFromURI converts a file URI to a local path. Other schemes, such as
// untitled buffers, have no package and are not checked.
func pathFromURI(uri string) (string, bool) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return "", false
	}
	path := parsed.Path
	if runtime.GOOS == "windows" {
		// file:///c:/dir/file.go has the path /c:/dir/file.go.
		path = strings.TrimPrefix(path, "/")
		if len(path) >= 2 && path[1] == ':' {
			path = strings.ToUpper(path[:1]) + path[1:]
		}
	}
	return filepath.Clean(filepath.FromSlash(path)), true
}

func uriFromPath(path string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}
