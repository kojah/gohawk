package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk"
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
			if analyzer.Examples.Flagged.Code == "" || analyzer.Examples.OK.Code == "" {
				t.Errorf("analyzer %q is missing generated examples", analyzer.Name)
			}
			if len(analyzer.Examples.Flagged.Diagnostics) == 0 || len(analyzer.Examples.OK.Diagnostics) != 0 {
				t.Errorf("analyzer %q example diagnostics = %d flagged, %d OK", analyzer.Name, len(analyzer.Examples.Flagged.Diagnostics), len(analyzer.Examples.OK.Diagnostics))
			}
		}
	}
	if analyzerCount != len(gohawk.Analyzers()) {
		t.Fatalf("analyzer count = %d, want %d", analyzerCount, len(gohawk.Analyzers()))
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
