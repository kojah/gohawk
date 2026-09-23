package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelperReferencesCoverContractsAndDetectDrift(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"syntax", "ssaflow", "summaries", "passes/newfacts"} {
		write("internal/"+name+"/api.go", "package "+filepath.Base(name)+`;
// State is a proof outcome.
type State int
const Unknown State = 0
var Default State
// Box keeps a value.
type Box[T any] struct { Value T; hidden bool }
// NewBox creates a box.
func NewBox[T any](value T) Box[T] { panic("body must not be copied") }
// Get reads the value.
func (box Box[T]) Get() T { return box.Value }
func hidden() {}
`)
	}
	write(helperIndexPage, "# Routing\n\n"+generatedHelpersStart+"\n"+generatedHelpersEnd+"\n")
	updates := make(map[string][]byte)
	if err := generateHelperReferences(root, updates); err != nil {
		t.Fatal(err)
	}
	for path, contents := range updates {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	page := filepath.Join(root, helperReferenceDirectory, "newfacts.md")
	text := string(updates[page])
	if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
		t.Fatal("reference must end in exactly one newline")
	}
	for _, want := range []string{
		"## Unknown", "## Default", "## Box", "## Box.Get", "## NewBox", "NewBox[T any](value T) Box[T]",
		"NewBox creates a box.", "../../../../internal/passes/newfacts/api.go)", "[the broker](summaries.md)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated reference lacks %q", want)
		}
	}
	if strings.Contains(text, "body must not be copied") || strings.Contains(text, "hidden bool") || strings.Contains(text, "func hidden") {
		t.Fatal("reference exposed bodies or private declarations")
	}
	if err := checkHelperReferences(root); err != nil {
		t.Fatalf("fresh references: %v", err)
	}
	write("internal/passes/newfacts/api.go", "package newfacts; func Added() {}")
	if err := checkHelperReferences(root); err == nil {
		t.Fatal("changed source was accepted without regeneration")
	}
	unchanged, err := os.ReadFile(page)
	if err != nil || string(unchanged) != text {
		t.Fatalf("freshness check changed the reference: %v", err)
	}
}
