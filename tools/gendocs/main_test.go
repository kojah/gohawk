package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/docexamples"
)

func TestGeneratedManifestMatchesCatalog(t *testing.T) {
	// generated-check owns the expensive live analyzer/example validation. This
	// unit test verifies the serialized catalog without repeating that full run.
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(root, "site", "src", "generated", "analyzers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data manifest
	if err := json.Unmarshal(contents, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Groups) != len(gohawk.AnalyzerGroups()) {
		t.Fatalf("group count = %d, want %d", len(data.Groups), len(gohawk.AnalyzerGroups()))
	}
	var analyzerCount int
	for _, group := range data.Groups {
		analyzerCount += len(group.Analyzers)
		for _, analyzer := range group.Analyzers {
			if !strings.HasPrefix(analyzer.Path, "analyzers/"+group.Slug+"/") {
				t.Errorf("analyzer %q path %q is outside group %q", analyzer.Name, analyzer.Path, group.Slug)
			}
			info := gohawk.AnalyzerMetadata()[analyzer.Name]
			if analyzer.Tier != info.Tier() {
				t.Errorf("analyzer %q tier metadata was not copied", analyzer.Name)
			}
			if len(analyzer.Checks) != len(info.Checks) {
				t.Errorf("analyzer %q check metadata was not copied", analyzer.Name)
			}
			for checkIndex, check := range analyzer.Checks {
				if check.ID == "" || check.Summary == "" || check.Kind == "" {
					t.Errorf("analyzer %q generated incomplete check metadata: %+v", analyzer.Name, check)
				}
				if check.Kind != info.Checks[checkIndex].Kind {
					t.Errorf("check %q kind = %q, want %q", check.ID, check.Kind, info.Checks[checkIndex].Kind)
				}
			}
		}
	}
	if analyzerCount != len(gohawk.Analyzers()) {
		t.Fatalf("analyzer count = %d, want %d", analyzerCount, len(gohawk.Analyzers()))
	}
}

func TestFastManifestLeavesExamplesUncollected(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := collectManifest(root, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range data.Groups {
		for _, analyzer := range group.Analyzers {
			if len(analyzer.Examples.Flagged) != 0 || analyzer.Examples.OK.Code != "" {
				t.Fatalf("fast manifest collected examples for %s", analyzer.Name)
			}
		}
	}
}

func TestDocsTimingsDescribePhasesAndCounts(t *testing.T) {
	timings := docsTimings{
		started:   time.Now(),
		examples:  true,
		analyzers: 2,
		pages:     4,
		collector: docexamples.Metrics{Targets: 2, Regions: 3, LoadedPackages: 5, AnalyzerRoots: 2},
	}
	output := timings.String()
	for _, want := range []string{
		"mode=examples analyzers=2 files=4",
		"fixture scan:", "targets=2 regions=3", "package load:", "packages=5",
		"analyzer run:", "roots=2", "page render:", "file sync:",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("timing output missing %q: %s", want, output)
		}
	}
}

func TestExamplesBlockTitlesMultipleFlaggedCases(t *testing.T) {
	block, err := examplesBlock(docexamples.Set{
		Flagged: []docexamples.Example{
			{Title: "First shape", Code: "func first() {}", Diagnostics: []docexamples.Diagnostic{{Message: "first"}}},
			{Title: "Second shape", Code: "func second() {}", Diagnostics: []docexamples.Diagnostic{{Message: "second"}}},
		},
		OK: docexamples.Example{Code: "func ok() {}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#### First shape", "#### Second shape", "### Accepted code"} {
		if !strings.Contains(block, want) {
			t.Fatalf("examples block is missing %q: %s", want, block)
		}
	}
	if !strings.Contains(block, "gohawk=\"") {
		t.Fatalf("examples block is missing diagnostic range metadata: %s", block)
	}
}

func TestGroupCardsUsesAnalyzerSummaryAndOmitsActivationMetadata(t *testing.T) {
	cards := groupCards(group{
		Slug: "reliability",
		Analyzers: []analyzer{{
			Name:    "example",
			Summary: "Checks the complete example problem.",
			Checks:  []check{{ID: "example/problem", Kind: "defect"}},
		}},
	})
	if !strings.Contains(cards, "Checks the complete example problem.") {
		t.Fatalf("catalog card lost analyzer summary: %s", cards)
	}
	if strings.Contains(cards, "analyzer-profile") || strings.Contains(cards, ">default<") {
		t.Fatalf("catalog card contains activation metadata: %s", cards)
	}
}

func TestSynchronizeGeneratedSections(t *testing.T) {
	tests := []struct {
		name        string
		contents    string
		synchronize func([]byte, string) ([]byte, error)
		want        string
	}{
		{
			name:        "examples markers",
			contents:    "# Rule\n\n## Examples\n\nold\n\n## Options\n",
			synchronize: synchronizeExamples,
			want:        "## Examples\n\n" + generatedExamplesStart + "\nnew\n" + generatedExamplesEnd + "\n\n## Options",
		},
		{
			name:        "checks subsection",
			contents:    "# Rule\n\n## What it detects\n\nSummary.\n\n## Why this is flagged\n",
			synchronize: synchronizeChecks,
			want:        "Summary.\n\n### Checks\n\n" + generatedChecksStart + "\nnew\n" + generatedChecksEnd + "\n\n## Why this is flagged",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.synchronize([]byte(test.contents), "new")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), test.want) {
				t.Fatalf("generated section missing from %q", got)
			}
		})
	}
}

func TestSynchronizeAnalyzerComponentsAddsImportsAfterFrontmatter(t *testing.T) {
	contents := []byte("---\ntitle: example\n---\n\n## What it detects\n")
	got, err := synchronizeAnalyzerComponents(contents)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: example\n---\n\n" + analyzerComponentImports + "\n\n## What it detects"
	if !strings.Contains(string(got), want) {
		t.Fatalf("component imports were not added after frontmatter: %s", got)
	}
}

func TestValidateAnalyzerFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name:     "valid",
			contents: "---\ntitle: example\nseoTitle: \"example: Detect example bugs in Go\"\n---\n",
		},
		{
			name:     "missing SEO title",
			contents: "---\ntitle: example\n---\n",
			wantErr:  "missing frontmatter seoTitle",
		},
		{
			name:     "wrong page title",
			contents: "---\ntitle: other\nseoTitle: \"example: Detect example bugs in Go\"\n---\n",
			wantErr:  `frontmatter title must be "example"`,
		},
		{
			name:     "wrong analyzer prefix",
			contents: "---\ntitle: example\nseoTitle: \"other: Detect example bugs in Go\"\n---\n",
			wantErr:  `must start with "example: "`,
		},
		{
			name:     "empty description",
			contents: "---\ntitle: example\nseoTitle: \"example:  \"\n---\n",
			wantErr:  "include a description",
		},
		{
			name:     "site suffix",
			contents: "---\ntitle: example\nseoTitle: \"example: Detect example bugs in Go | gohawk\"\n---\n",
			wantErr:  `must omit "| gohawk"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAnalyzerFrontmatter([]byte(test.contents), "example")
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateAnalyzerFrontmatter() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateAnalyzerFrontmatter() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestChecksBlockIncludesIDsDescriptionsAndTier(t *testing.T) {
	block, err := checksBlock("example", []check{{
		ID:      "example/problem",
		Summary: "Reports the example problem.",
		Kind:    "hazard",
		Tier:    gohawk.CheckTierExtended,
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| Check | Kind | Tier | What it detects |",
		"| <CheckIdentity name=\"problem\" tier=\"extended\" /> | hazard | extended |",
		"Reports the example problem.",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("checks block is missing %q: %s", want, block)
		}
	}
	if strings.Contains(block, "| `example/problem` |") {
		t.Fatalf("checks block repeats the analyzer prefix: %s", block)
	}
}

func TestChecksBlockOmitsDefaultActivation(t *testing.T) {
	block, err := checksBlock("example", []check{{
		ID:      "example/problem",
		Summary: "Reports the example problem.",
		Kind:    "policy",
		Tier:    gohawk.CheckTierCore,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(block, "optIn") || strings.Contains(block, "default") {
		t.Fatalf("default activation is visible in checks block: %s", block)
	}
}

func TestChecksBlockRejectsMismatchedAnalyzerPrefix(t *testing.T) {
	_, err := checksBlock("example", []check{{ID: "different/problem"}})
	if err == nil {
		t.Fatal("checks block accepted a mismatched analyzer prefix")
	}
}

func TestSynchronizeOptionsAddsSection(t *testing.T) {
	got, err := synchronizeOptions([]byte("# Rule\n"), "| Knob |\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "## Options\n\n"+generatedOptionsStart+"\n| Knob |\n"+generatedOptionsEnd) {
		t.Fatalf("generated options section missing from %q", got)
	}
}

func TestReplaceGeneratedBlock(t *testing.T) {
	contents := []byte("before\nSTART\nold\nEND\nafter\n")
	got, err := replaceGeneratedBlock(contents, "START", "END", "new")
	if err != nil {
		t.Fatal(err)
	}
	want := "before\nSTART\nnew\nEND\nafter\n"
	if string(got) != want {
		t.Fatalf("replacement = %q, want %q", got, want)
	}
}

func TestUpdateFileCheckRejectsStaleContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "generated.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := updateFile(root, path, []byte("new\n"), true); err == nil {
		t.Fatal("check accepted stale generated content")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "old\n" {
		t.Fatalf("check modified file to %q", contents)
	}
}
