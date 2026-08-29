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
		"warning[example]: problem found",
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
