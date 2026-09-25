package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePosition(t *testing.T) {
	position, err := parsePosition(`C:\work\sample.go:12:7`)
	if err != nil {
		t.Fatal(err)
	}
	if position.Filename != `C:\work\sample.go` || position.Line != 12 || position.Column != 7 {
		t.Fatalf("position = %#v", position)
	}
}

func TestRenderDiagnosticUsesFullRange(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "sample.go")
	if err := os.WriteFile(filename, []byte("package sample\n\nfunc f() {\n\tproblem()\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	renderDiagnostic(&output, positionedDiagnostic{
		Analyzer: "example",
		Start:    sourcePosition{Filename: filename, Line: 4, Column: 2},
		End:      sourcePosition{Filename: filename, Line: 4, Column: 11},
		Message:  "problem found",
	}, 0, colorPalette{})

	for _, want := range []string{
		"warning: problem found [example]",
		filename + ":4:2",
		"4 | \tproblem()",
		"| \t^~~~~~~~~",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestDecodeDiagnosticsDeduplicatesTestVariants(t *testing.T) {
	data := []byte(`{
		"example.com/sample": {"rule": [{"posn":"sample.go:3:2","end":"sample.go:3:6","message":"problem"}]},
		"example.com/sample [example.com/sample.test]": {"rule": [{"posn":"sample.go:3:2","end":"sample.go:3:6","message":"problem"}]}
	}`)
	diagnostics, analysisErrors, err := decodeDiagnostics(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysisErrors) != 0 || len(diagnostics) != 1 {
		t.Fatalf("got %d diagnostics and %d errors", len(diagnostics), len(analysisErrors))
	}
}

func TestDocumentationFooterLinksEachAnalyzerOnce(t *testing.T) {
	var output strings.Builder
	renderDocumentationFooter(&output, []positionedDiagnostic{
		{Analyzer: "lockorder"}, {Analyzer: "resourcelifetime"}, {Analyzer: "lockorder"}, {Analyzer: "unknown"},
	})
	want := "\nLearn more about these findings:\n" +
		"  lockorder: https://gohawk.dev/analyzers/concurrency-and-synchronization/lockorder/\n" +
		"  resourcelifetime: https://gohawk.dev/analyzers/resources-and-lifecycle/resourcelifetime/\n"
	if output.String() != want {
		t.Fatalf("footer = %q, want %q", output.String(), want)
	}
	output.Reset()
	renderDocumentationFooter(&output, nil)
	if output.Len() != 0 {
		t.Fatalf("footer without diagnostics = %q, want nothing", output.String())
	}
}

func TestRenderDiagnosticDrawsLabeledEvidence(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "sample.go")
	source := "package sample\n\nfunc f() {\n\topen()\n\treturn\n}\n"
	if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	renderDiagnostic(&output, positionedDiagnostic{
		Analyzer: "example",
		Check:    "example/leak",
		Start:    sourcePosition{Filename: filename, Line: 4, Column: 2},
		End:      sourcePosition{Filename: filename, Line: 4, Column: 8},
		Message:  "leaked",
		Related: []jsonRelated{
			{Posn: filename + ":5:2", End: filename + ":5:8", Message: "returns here without releasing it"},
			{Posn: "elsewhere.go:9:1", Message: "a location with no readable source"},
		},
	}, 0, colorPalette{})
	for _, want := range []string{
		"warning: leaked [example/leak]",
		"4 | \topen()",
		"5 | \treturn",
		"| \t^~~~~~ returns here without releasing it",
		"= note: a location with no readable source",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestRenderDiagnosticKeepsOverlappingMarkers(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "sample.go")
	source := "package sample\n\nfunc f() {\n\tfor {\n\t\tdefer g()\n\t}\n}\n"
	if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	renderDiagnostic(&output, positionedDiagnostic{
		Analyzer: "example",
		Start:    sourcePosition{Filename: filename, Line: 5, Column: 3},
		End:      sourcePosition{Filename: filename, Line: 5, Column: 12},
		Message:  "deferred in a loop",
		Related: []jsonRelated{
			{Posn: filename + ":4:2", End: filename + ":6:3", Message: "the whole loop"},
		},
	}, 0, colorPalette{})
	text := output.String()
	if strings.Count(text, "5 | \t\tdefer g()") != 1 {
		t.Errorf("line 5 should print once:\n%s", text)
	}
	if !strings.Contains(text, "| \t\t^~~~~~~~~\n") || !strings.Contains(text, "the whole loop") {
		t.Errorf("both the primary marker and the evidence label should appear:\n%s", text)
	}
}
