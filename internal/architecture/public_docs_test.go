package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The site publishes docs/ except docs/development/. Public pages say what a
// user can rely on; precision boundaries, proof mechanics, and dogfood
// evidence belong in the development reference. These budgets keep that
// detail from drifting back onto the site one paragraph at a time.
const (
	publicAnalyzerPageBudget = 130
	publicPageBudget         = 200
)

// publicPageBaseline records pages that were already over budget when the
// budget was introduced. They may shrink but not grow.
var publicPageBaseline = map[string]int{
	"docs/index.md":             242,
	"docs/understanding-ssa.md": 263,
}

// A commit-pinned link to a dogfooded repository is evidence for a precision
// decision, which is development material.
var pinnedSourceLink = regexp.MustCompile(`github\.com/[^\s)]+/blob/[0-9a-f]{40}`)

func TestPublicDocumentationStaysConcise(t *testing.T) {
	t.Parallel()
	root := newRepositorySourceInventory(t).root
	docs := filepath.Join(root, "docs")
	err := filepath.WalkDir(docs, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path == filepath.Join(docs, "development") {
			return filepath.SkipDir
		}
		if entry.IsDir() || (filepath.Ext(path) != ".md" && filepath.Ext(path) != ".mdx") {
			return nil
		}
		relative := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		lines := strings.Count(text, "\n")
		budget := publicPageBudget
		if strings.HasPrefix(relative, "docs/analyzers/") {
			budget = publicAnalyzerPageBudget
		}
		if baseline, ok := publicPageBaseline[relative]; ok {
			budget = baseline
		}
		if lines > budget {
			t.Errorf("%s has %d lines, over its budget of %d; move detail to docs/development/", relative, lines, budget)
		}
		if link := pinnedSourceLink.FindString(text); link != "" {
			t.Errorf("%s links pinned source %s; dogfood evidence belongs in docs/development/", relative, link)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
