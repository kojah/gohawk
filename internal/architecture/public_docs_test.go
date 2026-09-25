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
	// Generated tables and examples make up most of an analyzer page, so the
	// hand-written prose has its own, tighter budget.
	analyzerProseBudget     = 40
	analyzerParagraphBudget = 8
)

// publicPageBaseline records pages that were already over budget when the
// budget was introduced. They may shrink but not grow.
var publicPageBaseline = map[string]int{
	"docs/index.md":             242,
	"docs/understanding-ssa.md": 206,
}

// A commit-pinned link to a dogfooded repository is evidence for a precision
// decision, which is development material.
var pinnedSourceLink = regexp.MustCompile(`github\.com/[^\s)]+/blob/[0-9a-f]{40}`)

// hedgingVocabulary is how a proof boundary reads when it is written for
// the analyzer's author rather than its user: what the proof declines, where
// evidence is opaque, what an accepted case is not proof of. On an analyzer
// page, a user needs what is reported and what is deliberately left alone;
// the reasoning behind each boundary goes in the analyzer's design note.
var hedgingVocabulary = regexp.MustCompile(`(?i)\b(` +
	`uncertain(ty)?|opaque|boundary|declines?|conservative(ly)?|` +
	`does not prove|is not proof|not proof of|does not establish)\b`)

var (
	generatedBlock = regexp.MustCompile(`(?s)\{/\* gohawk:generated-[a-z]+:start \*/\}.*?\{/\* gohawk:generated-[a-z]+:end \*/\}`)
	fencedCode     = regexp.MustCompile("(?s)```.*?```")
	frontMatter    = regexp.MustCompile(`(?s)\A---\n.*?\n---\n`)
	inlineCode     = regexp.MustCompile("`[^`]*`")
)

// analyzerProse returns the hand-written paragraphs of an analyzer page:
// generated blocks, code, front matter, headings, imports, and tables removed.
func analyzerProse(text string) [][]string {
	text = generatedBlock.ReplaceAllString(text, "")
	text = fencedCode.ReplaceAllString(text, "")
	text = frontMatter.ReplaceAllString(text, "")
	var paragraphs [][]string
	for _, block := range regexp.MustCompile(`\n\s*\n`).Split(text, -1) {
		var lines []string
		for line := range strings.SplitSeq(block, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "|") {
				continue
			}
			lines = append(lines, trimmed)
		}
		if len(lines) > 0 {
			paragraphs = append(paragraphs, lines)
		}
	}
	return paragraphs
}

// checkAnalyzerProse applies the analyzer page rules to its hand-written prose.
func checkAnalyzerProse(t *testing.T, relative, text string) {
	t.Helper()
	total := 0
	for _, paragraph := range analyzerProse(text) {
		total += len(paragraph)
		if len(paragraph) > analyzerParagraphBudget {
			t.Errorf("%s has a %d-line paragraph starting %q; keep paragraphs to %d lines", relative, len(paragraph), paragraph[0], analyzerParagraphBudget)
		}
		prose := inlineCode.ReplaceAllString(strings.Join(paragraph, " "), "")
		if word := hedgingVocabulary.FindString(prose); word != "" {
			t.Errorf("%s says %q; describe what is reported and move the proof boundary to its design note", relative, word)
		}
	}
	if total > analyzerProseBudget {
		t.Errorf("%s has %d lines of hand-written prose, over %d; move detail to its design note", relative, total, analyzerProseBudget)
	}
}

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
		if strings.HasPrefix(relative, "docs/analyzers/") && filepath.Ext(path) == ".mdx" {
			checkAnalyzerProse(t, relative, text)
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

func TestAnalyzerProseRulesSeparateVentingFromContracts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		text string
		want bool
	}{
		{"proof boundary", "A helper that closes the body is an uncertainty boundary.", true},
		{"declined claim", "The summary path declines branching helpers.", true},
		{"not proof", "This is uncertainty about cleanup, not proof of a cancel call.", true},
		{"design note link", "The full list of precision boundaries is in the design notes.", false},
		{"inline code", "Set `-uncertain-mode` to change the report.", false},
		{"contract", "Reports sends reachable after the channel has been closed.", false},
	} {
		got := false
		for _, paragraph := range analyzerProse(test.text) {
			prose := inlineCode.ReplaceAllString(strings.Join(paragraph, " "), "")
			got = got || hedgingVocabulary.MatchString(prose)
		}
		if got != test.want {
			t.Errorf("%s: flagged = %v, want %v", test.name, got, test.want)
		}
	}
}
