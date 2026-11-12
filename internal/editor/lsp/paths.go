package lsp

import (
	"path/filepath"
)

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
