package docexamples

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRegions(t *testing.T) {
	source := []byte("package p\n\n//gohawk:example flagged Direct failure\nfunc bad() {} // want \"problem\"\n//gohawk:example end\n")
	regions, err := parseRegions("example.go", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 {
		t.Fatalf("region count = %d, want 1", len(regions))
	}
	if got, want := regions[0].code, "func bad() {}"; got != want {
		t.Fatalf("display code = %q, want %q", got, want)
	}
	if got, want := regions[0].title, "Direct failure"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}

func TestReadRegionsAllowsMultipleFlagged(t *testing.T) {
	directory := t.TempDir()
	source := []byte("package p\n\n//gohawk:example flagged First\nfunc first() {}\n//gohawk:example end\n\n//gohawk:example flagged Second\nfunc second() {}\n//gohawk:example end\n\n//gohawk:example ok\nfunc ok() {}\n//gohawk:example end\n")
	if err := os.WriteFile(filepath.Join(directory, "examples.go"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	regions, _, err := readRegions(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(regions), 3; got != want {
		t.Fatalf("region count = %d, want %d", got, want)
	}
}

func TestParseRegionsRejectsUnclosedMarker(t *testing.T) {
	_, err := parseRegions("example.go", []byte("//gohawk:example ok\nfunc ok() {}\n"))
	if err == nil || !strings.Contains(err.Error(), "unclosed") {
		t.Fatalf("error = %v, want unclosed marker error", err)
	}
}
