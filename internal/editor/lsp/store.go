package lsp

import (
	"path/filepath"
	"sort"
)

// The last findings per package directory. A package that was not re-checked
// keeps its previous findings; a package that stopped compiling keeps them
// too, marked stale, because the positions may no longer be right.

type resultStore struct {
	byDirectory map[string][]fileDiagnostic
	stale       map[string]bool
}

func newResultStore() *resultStore {
	return &resultStore{byDirectory: make(map[string][]fileDiagnostic), stale: make(map[string]bool)}
}

// replace installs fresh findings for the covered directories and returns the
// files whose published list must be sent again, including files that lost
// their last finding.
func (s *resultStore) replace(covered []string, fresh []fileDiagnostic) []string {
	touched := make(map[string]bool)
	for _, directory := range covered {
		for _, previous := range s.byDirectory[directory] {
			touched[previous.path] = true
		}
		delete(s.byDirectory, directory)
		delete(s.stale, directory)
	}
	coveredSet := make(map[string]bool, len(covered))
	for _, directory := range covered {
		coveredSet[directory] = true
	}
	for _, finding := range fresh {
		directory := filepath.Dir(finding.path)
		if !coveredSet[directory] {
			// A finding outside the requested packages would overwrite
			// results this run did not recompute in full.
			continue
		}
		s.byDirectory[directory] = append(s.byDirectory[directory], finding)
		touched[finding.path] = true
	}
	return sortedKeys(touched)
}

// directoriesUnder lists stored directories inside root, together with the
// directories of fresh findings, which is what a workspace run covers.
func (s *resultStore) directoriesUnder(root string, fresh []fileDiagnostic) []string {
	covered := make(map[string]bool)
	for directory := range s.byDirectory {
		if insideDirectory(root, directory) {
			covered[directory] = true
		}
	}
	for _, finding := range fresh {
		if directory := filepath.Dir(finding.path); insideDirectory(root, directory) {
			covered[directory] = true
		}
	}
	return sortedKeys(covered)
}

func (s *resultStore) markStale(directories []string) {
	for _, directory := range directories {
		if len(s.byDirectory[directory]) > 0 {
			s.stale[directory] = true
		}
	}
}

func (s *resultStore) clear() []string {
	touched := make(map[string]bool)
	for _, findings := range s.byDirectory {
		for _, finding := range findings {
			touched[finding.path] = true
		}
	}
	s.byDirectory = make(map[string][]fileDiagnostic)
	s.stale = make(map[string]bool)
	return sortedKeys(touched)
}

// forFile returns the findings to publish for one file, in position order.
func (s *resultStore) forFile(path string) []diagnostic {
	diagnostics := []diagnostic{}
	for _, finding := range s.byDirectory[filepath.Dir(path)] {
		if finding.path == path {
			diagnostics = append(diagnostics, finding.diagnostic)
		}
	}
	sort.SliceStable(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i].Range.Start, diagnostics[j].Range.Start
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Character < b.Character
	})
	return diagnostics
}

func (s *resultStore) total() int {
	total := 0
	for _, findings := range s.byDirectory {
		total += len(findings)
	}
	return total
}

func (s *resultStore) anyStale() bool {
	return len(s.stale) > 0
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
