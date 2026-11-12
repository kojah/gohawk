package lsp

import (
	"path/filepath"
	"sort"
)

// The last findings per package directory, and the directories whose
// findings are currently unknown. A package that was not re-checked keeps its
// previous findings. A package whose last check could not finish (it stopped
// compiling, or the run was stopped at a limit) keeps them too, and is
// recorded as unresolved until a check of it completes, so its state is never
// reported as clean.

type resultStore struct {
	byDirectory map[string][]fileDiagnostic
	unresolved  map[string]runOutcome
}

func newResultStore() *resultStore {
	return &resultStore{byDirectory: make(map[string][]fileDiagnostic), unresolved: make(map[string]runOutcome)}
}

// replace installs fresh findings for the covered directories. It returns the
// files whose published list must be sent again, including files that lost
// their last finding, and the findings it refused because they lie outside
// the covered directories.
func (s *resultStore) replace(covered []string, fresh []fileDiagnostic) (touchedFiles []string, refused []fileDiagnostic) {
	touched := make(map[string]bool)
	for _, directory := range covered {
		for _, previous := range s.byDirectory[directory] {
			touched[previous.path] = true
		}
		delete(s.byDirectory, directory)
		delete(s.unresolved, directory)
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
			refused = append(refused, finding)
			continue
		}
		s.byDirectory[directory] = append(s.byDirectory[directory], finding)
		touched[finding.path] = true
	}
	return sortedKeys(touched), refused
}

// directoriesOfModule lists the stored directories that belong to the module
// at root. A nested module's directories are not part of it: a run over the
// outer module does not analyze them and must not clear their findings.
func (s *resultStore) directoriesOfModule(root string) []string {
	owned := make(map[string]bool)
	for directory := range s.byDirectory {
		if module, ok := moduleRoot(directory); ok && module == root {
			owned[directory] = true
		}
	}
	for directory := range s.unresolved {
		if module, ok := moduleRoot(directory); ok && module == root {
			owned[directory] = true
		}
	}
	return sortedKeys(owned)
}

// markUnresolved records that the findings of directories are unknown.
func (s *resultStore) markUnresolved(directories []string, outcome runOutcome) {
	for _, directory := range directories {
		s.unresolved[directory] = outcome
	}
}

// dropUnder removes everything stored for a deleted file or directory and
// returns the files to publish again.
func (s *resultStore) dropUnder(path string) []string {
	touched := make(map[string]bool)
	for directory, findings := range s.byDirectory {
		kept := findings[:0:0]
		for _, finding := range findings {
			if finding.path == path || insideDirectory(path, finding.path) {
				touched[finding.path] = true
				continue
			}
			kept = append(kept, finding)
		}
		if len(kept) == 0 {
			delete(s.byDirectory, directory)
		} else {
			s.byDirectory[directory] = kept
		}
	}
	for directory := range s.unresolved {
		if directory == path || insideDirectory(path, directory) {
			delete(s.unresolved, directory)
		}
	}
	return sortedKeys(touched)
}

// dropWhere removes the findings matching unwanted and returns the files to
// publish again.
func (s *resultStore) dropWhere(unwanted func(diagnostic) bool) []string {
	touched := make(map[string]bool)
	for directory, findings := range s.byDirectory {
		kept := findings[:0:0]
		for _, finding := range findings {
			if unwanted(finding.diagnostic) {
				touched[finding.path] = true
				continue
			}
			kept = append(kept, finding)
		}
		if len(kept) == 0 {
			delete(s.byDirectory, directory)
		} else {
			s.byDirectory[directory] = kept
		}
	}
	return sortedKeys(touched)
}

func (s *resultStore) clear() []string {
	touched := make(map[string]bool)
	for _, findings := range s.byDirectory {
		for _, finding := range findings {
			touched[finding.path] = true
		}
	}
	s.byDirectory = make(map[string][]fileDiagnostic)
	s.unresolved = make(map[string]runOutcome)
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

// unresolvedBy lists the directories left unresolved by one of the outcomes,
// and how many stored findings they hold.
func (s *resultStore) unresolvedBy(outcomes ...runOutcome) (directories []string, findings int) {
	for directory, outcome := range s.unresolved {
		for _, wanted := range outcomes {
			if outcome == wanted {
				directories = append(directories, directory)
				findings += len(s.byDirectory[directory])
			}
		}
	}
	sort.Strings(directories)
	return directories, findings
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
