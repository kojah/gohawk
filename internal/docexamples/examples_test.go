package docexamples

import (
	"strings"
	"testing"
)

func TestParseRegions(t *testing.T) {
	source := []byte("package p\n\n//gohawk:example flagged\nfunc bad() {} // want \"problem\"\n//gohawk:example end\n")
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
}

func TestParseRegionsRejectsUnclosedMarker(t *testing.T) {
	_, err := parseRegions("example.go", []byte("//gohawk:example ok\nfunc ok() {}\n"))
	if err == nil || !strings.Contains(err.Error(), "unclosed") {
		t.Fatalf("error = %v, want unclosed marker error", err)
	}
}
