package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
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
		"  resourcelifetime: https://gohawk.dev/analyzers/resources-and-lifecycle/resourcelifetime/\n" +
		"To see the full reasoning behind a finding, rerun with\n" +
		"  -gohawk-trace=<analyzer> -gohawk-trace-candidate=<file:line>\n"
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
	text := renderSample(t, "package sample\n\nfunc f() {\n\topen()\n\treturn\n}\n", func(filename string) positionedDiagnostic {
		return positionedDiagnostic{
			Analyzer: "example",
			Check:    "example/leak",
			Start:    sourcePosition{Filename: filename, Line: 4, Column: 2},
			End:      sourcePosition{Filename: filename, Line: 4, Column: 8},
			Message:  "leaked",
			Related: []jsonRelated{
				{Posn: filename + ":5:2", End: filename + ":5:8", Message: "returns here without releasing it"},
				{Posn: "elsewhere.go:9:1", Message: "a location with no readable source"},
			},
		}
	})
	for _, want := range []string{
		"warning: leaked [example/leak]",
		"4 | \topen()",
		"5 | \treturn",
		"| \t^~~~~~ returns here without releasing it",
		"= note: a location with no readable source",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output does not contain %q:\n%s", want, text)
		}
	}
}

func TestRenderDiagnosticPrintsCheckHelp(t *testing.T) {
	var output bytes.Buffer
	renderDiagnostic(&output, positionedDiagnostic{
		Analyzer: "resourcelifetime",
		Check:    "resourcelifetime/missing-release",
		Start:    sourcePosition{Filename: "missing.go", Line: 1, Column: 1},
		Message:  "leaked",
	}, 0, colorPalette{})
	if !strings.Contains(output.String(), "= help: release it on every return path") {
		t.Errorf("help line missing:\n%s", output.String())
	}
}

func TestRenderDiagnosticSpanLayout(t *testing.T) {
	type span struct{ line, column, endLine, endColumn int }
	for _, test := range []struct {
		name    string
		source  string
		primary span
		related []jsonRelated
		want    []string
		reject  []string
		markers int
	}{
		{
			name:    "an overlapping span keeps both markers",
			source:  "package sample\n\nfunc f() {\n\tfor {\n\t\tdefer g()\n\t}\n}\n",
			primary: span{5, 3, 5, 12},
			related: []jsonRelated{{Posn: ":4:2", End: ":6:3", Message: "the whole loop"}},
			want:    []string{"| \t\t^~~~~~~~~\n", "the whole loop"},
			markers: 2,
		},
		{
			name:    "a block is marked on its first line",
			source:  "package sample\n\nfunc f() {\n\tgo func() {\n\t\twork()\n\t}()\n\treturn\n}\n",
			primary: span{4, 2, 6, 5},
			related: []jsonRelated{{Posn: ":7:2", End: ":7:8", Message: "returns here"}},
			want:    []string{"4 | \tgo func() {\n  | \t^~~~~~~~~~~\n"},
			reject:  []string{"work()"},
			markers: 2,
		},
		{
			name:    "evidence at the reported span labels its marker",
			source:  "package sample\n\nfunc f() {\n\tb.Lock()\n}\n",
			primary: span{4, 2, 4, 10},
			related: []jsonRelated{{Posn: ":4:2", End: ":4:10", Message: "then `b` is locked"}},
			want:    []string{"^~~~~~~~ then `b` is locked"},
			markers: 1,
		},
	} {
		text := renderSample(t, test.source, func(filename string) positionedDiagnostic {
			related := slices.Clone(test.related)
			for index := range related {
				related[index].Posn = filename + related[index].Posn
				related[index].End = filename + related[index].End
			}
			return positionedDiagnostic{
				Analyzer: "example",
				Start:    sourcePosition{Filename: filename, Line: test.primary.line, Column: test.primary.column},
				End:      sourcePosition{Filename: filename, Line: test.primary.endLine, Column: test.primary.endColumn},
				Message:  test.name,
				Related:  related,
			}
		})
		for _, want := range test.want {
			if !strings.Contains(text, want) {
				t.Errorf("%s: output does not contain %q:\n%s", test.name, want, text)
			}
		}
		for _, reject := range test.reject {
			if strings.Contains(text, reject) {
				t.Errorf("%s: output contains %q:\n%s", test.name, reject, text)
			}
		}
		if got := strings.Count(text, "^"); got != test.markers {
			t.Errorf("%s: %d markers, want %d:\n%s", test.name, got, test.markers, text)
		}
	}
}

// renderSample writes source to a file and renders the diagnostic that
// diagnostic builds for it, without colors or context lines.
func renderSample(t *testing.T, source string, diagnostic func(filename string) positionedDiagnostic) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	renderDiagnostic(&output, diagnostic(filename), 0, colorPalette{})
	return output.String()
}
