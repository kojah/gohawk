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
			if len(analyzer.Tags) != len(info.Tags) {
				t.Errorf("analyzer %q tag metadata was not copied", analyzer.Name)
			}
		}
	}
	if analyzerCount != len(gohawk.Analyzers()) {
		t.Fatalf("analyzer count = %d, want %d", analyzerCount, len(gohawk.Analyzers()))
	}
}

func TestExamplesBlockTitlesMultipleFlaggedCases(t *testing.T) {
	block := examplesBlock(docexamples.Set{
		Flagged: []docexamples.Example{
			{Title: "First shape", Code: "func first() {}", Diagnostics: []docexamples.Diagnostic{{Message: "first"}}},
			{Title: "Second shape", Code: "func second() {}", Diagnostics: []docexamples.Diagnostic{{Message: "second"}}},
		},
		OK: docexamples.Example{Code: "func ok() {}"},
	})
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
			Tags:    []string{"reliability"},
		}},
	})
	if !strings.Contains(cards, "Checks the complete example problem.") {
		t.Fatalf("catalog card lost analyzer summary: %s", cards)
	}
	if strings.Contains(cards, "analyzer-profile") || strings.Contains(cards, ">default<") {
		t.Fatalf("catalog card contains analyzer profile: %s", cards)
	}
	if !strings.Contains(cards, `class="analyzer-tag"`) {
		t.Fatalf("catalog card lost analyzer tags: %s", cards)
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
