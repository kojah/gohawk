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

// unsavedTexts returns the text of every unsaved Go buffer accepted by wanted,
// by file path. Only Go source can change what the analysis sees; copying
// other unsaved buffers into every run would cost disk and memory for nothing.
func (s *documentStore) unsavedTexts(wanted func(path string) bool) map[string]string {
	texts := make(map[string]string)
	for path, current := range s.byPath {
		if current.unsaved && filepath.Ext(path) == ".go" && wanted(path) {
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

// The editor and the go command can spell one file two ways: the editor uses
// the path the user opened, and the go command may report the path with
// symbolic links resolved. Findings are stored and published under the
// editor's spelling, because that is the document the user has open; a
// finding filed under the other spelling would never be shown.

type spelledRoot struct {
	editor   string // the workspace folder as the editor names it
	resolved string // the same folder with symbolic links resolved
}

type pathSpelling struct {
	roots    []spelledRoot
	resolved map[string]string // directory to its resolved form
}

func newPathSpelling(workspaceRoots []string) *pathSpelling {
	spelling := &pathSpelling{resolved: make(map[string]string)}
	for _, root := range workspaceRoots {
		spelling.roots = append(spelling.roots, spelledRoot{editor: root, resolved: spelling.resolve(root)})
	}
	return spelling
}

// resolve follows symbolic links, and returns the cleaned input when the path
// does not exist or cannot be read.
func (p *pathSpelling) resolve(path string) string {
	path = filepath.Clean(path)
	if resolved, ok := p.resolved[path]; ok {
		return resolved
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	p.resolved[path] = resolved
	return resolved
}

// editorPath returns path as the editor spells it: a path inside a workspace
// folder is rebased onto that folder's own spelling. Other paths are returned
// unchanged.
func (p *pathSpelling) editorPath(path string) string {
	if p == nil {
		return path
	}
	path = filepath.Clean(path)
	directory := p.resolve(filepath.Dir(path))
	for _, root := range p.roots {
		for _, form := range []string{root.resolved, root.editor} {
			if !insideDirectory(form, directory) {
				continue
			}
			relative, err := filepath.Rel(form, directory)
			if err != nil {
				continue
			}
			return filepath.Join(root.editor, relative, filepath.Base(path))
		}
	}
	return path
}
