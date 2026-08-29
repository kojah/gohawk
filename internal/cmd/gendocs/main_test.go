package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/docexamples"
)

func TestCollectManifest(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := collectManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Groups) != len(gohawk.AnalyzerGroups()) {
		t.Fatalf("group count = %d, want %d", len(data.Groups), len(gohawk.AnalyzerGroups()))
	}
	if len(data.Tags) != len(gohawk.TagCatalog()) {
		t.Fatalf("tag count = %d, want %d", len(data.Tags), len(gohawk.TagCatalog()))
	}
	for _, tag := range data.Tags {
		if tag.ID == "" || tag.Description == "" {
			t.Errorf("generated tag is incomplete: %+v", tag)
		}
	}
	var analyzerCount int
	for _, group := range data.Groups {
		analyzerCount += len(group.Analyzers)
		for _, analyzer := range group.Analyzers {
			if !strings.HasPrefix(analyzer.Path, "analyzers/"+group.Slug+"/") {
				t.Errorf("analyzer %q path %q is outside group %q", analyzer.Name, analyzer.Path, group.Slug)
			}
			if len(analyzer.Examples.Flagged) == 0 || analyzer.Examples.OK.Code == "" {
				t.Errorf("analyzer %q is missing generated examples", analyzer.Name)
			}
			for index, example := range analyzer.Examples.Flagged {
				if len(example.Diagnostics) == 0 {
					t.Errorf("analyzer %q flagged example %d has no diagnostics", analyzer.Name, index+1)
				}
			}
			if len(analyzer.Examples.OK.Diagnostics) != 0 {
				t.Errorf("analyzer %q OK example has %d diagnostics", analyzer.Name, len(analyzer.Examples.OK.Diagnostics))
			}
			info := gohawk.AnalyzerMetadata()[analyzer.Name]
			if analyzer.Profile != string(info.Profile) {
				t.Errorf("analyzer %q profile metadata was not copied", analyzer.Name)
			}
			if len(analyzer.Checks) != len(info.Checks) {
				t.Errorf("analyzer %q check metadata was not copied", analyzer.Name)
			}
			for _, check := range analyzer.Checks {
				if check.ID == "" || check.Summary == "" || check.Profile == "" || len(check.Tags) == 0 {
					t.Errorf("analyzer %q generated incomplete check metadata: %+v", analyzer.Name, check)
				}
			}
		}
	}
	if analyzerCount != len(gohawk.Analyzers()) {
		t.Fatalf("analyzer count = %d, want %d", analyzerCount, len(gohawk.Analyzers()))
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

func TestGroupCardsUsesAnalyzerSummaryAndOmitsProfile(t *testing.T) {
	cards := groupCards(group{
		Slug: "reliability",
		Analyzers: []analyzer{{
			Name:    "example",
			Summary: "Checks the complete example problem.",
			Profile: "default",
			Checks:  []check{{ID: "example/problem", Tags: []string{"reliability"}}},
		}},
	})
	if !strings.Contains(cards, "Checks the complete example problem.") {
		t.Fatalf("catalog card lost analyzer summary: %s", cards)
	}
	if strings.Contains(cards, "analyzer-profile") || strings.Contains(cards, ">default<") {
		t.Fatalf("catalog card contains analyzer profile: %s", cards)
	}
	if strings.Contains(cards, `class="analyzer-tag"`) {
		t.Fatalf("catalog card contains projected analyzer tags: %s", cards)
	}
}

func TestSynchronizeExamplesAddsGeneratedMarkers(t *testing.T) {
	contents := []byte("# Rule\n\n## Examples\n\nold\n\n## Options\n")
	got, err := synchronizeExamples(contents, "new")
	if err != nil {
		t.Fatal(err)
	}
	want := "## Examples\n\n" + generatedExamplesStart + "\nnew\n" + generatedExamplesEnd + "\n\n## Options"
	if !strings.Contains(string(got), want) {
		t.Fatalf("generated examples section missing from %q", got)
	}
}

func TestSynchronizeChecksAddsGeneratedSubsection(t *testing.T) {
	contents := []byte("# Rule\n\n## What it detects\n\nSummary.\n\n## Why this is flagged\n")
	got, err := synchronizeChecks(contents, "checks")
	if err != nil {
		t.Fatal(err)
	}
	want := "Summary.\n\n### Checks\n\n" + generatedChecksStart + "\nchecks\n" + generatedChecksEnd + "\n\n## Why this is flagged"
	if !strings.Contains(string(got), want) {
		t.Fatalf("generated checks subsection missing from %q", got)
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

func TestChecksBlockIncludesIDsDescriptionsAndTagComponents(t *testing.T) {
	block, err := checksBlock([]check{{
		ID:      "example/problem",
		Summary: "Reports the example problem.",
		Profile: "opt-in",
		Tags:    []string{"correctness", "reliability"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| Check | What it detects | Profile | Tags |",
		"| `example/problem` |",
		"Reports the example problem.",
		`<CheckProfile profile="opt-in" />`,
		`<CheckTags tags={["correctness","reliability"]} />`,
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("checks block is missing %q: %s", want, block)
		}
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
