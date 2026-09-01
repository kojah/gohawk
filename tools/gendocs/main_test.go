package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			if analyzer.OptIn != info.OptIn {
				t.Errorf("analyzer %q opt-in metadata was not copied", analyzer.Name)
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

func TestChecksBlockIncludesIDsDescriptionsAndOptInMarker(t *testing.T) {
	block, err := checksBlock("example", []check{{
		ID:      "example/problem",
		Summary: "Reports the example problem.",
		Kind:    "hazard",
		OptIn:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| Check | Kind | What it detects |",
		"| <CheckIdentity name=\"problem\" optIn /> | hazard |",
		"Reports the example problem.",
		`\* Opt-in; requires explicit selection.`,
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
