package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintFacts(t *testing.T) {
	directory := t.TempDir()
	writeFile := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("go.mod", "module example.com/factsdump\n\ngo 1.25\n")
	writeFile("lib.go", `package factsdump

import "os"

// CloseFile closes its parameter on every return.
func CloseFile(file *os.File) error { return file.Close() }

// MaybeClose closes only when enabled.
func MaybeClose(file *os.File, enabled bool) {
	if enabled {
		_ = file.Close()
	}
}

// Open forwards os.Open, so its file is nil whenever its error is not.
func Open(path string) (*os.File, error) { return os.Open(path) }

// Finish closes the channel it is handed.
func Finish(done chan struct{}) { close(done) }
`)
	t.Chdir(directory)
	var output, errorsOutput bytes.Buffer
	if err := printFacts([]string{"."}, &output, &errorsOutput); err != nil {
		t.Fatalf("printFacts() error = %v, stderr %s", err, errorsOutput.String())
	}
	text := output.String()
	for _, want := range []string{
		"CloseFile (exported here", "0 file: Closed", "MaybeClose (exported here", "Close parameter 0 when argument 1 is true",
		"gohawkresultfacts example.com/factsdump.Open", "result 0 (*os.File) is nil when result 1 is non-nil",
		"gohawkconcurrencyfacts example.com/factsdump.Finish", "close parameter 0",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("facts dump lacks %q:\n%s", want, text)
		}
	}
}

func TestPrintFactsSelectsKinds(t *testing.T) {
	directory := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/factkinds\n\ngo 1.25\n",
		"lib.go": `package factkinds

import "os"

// Open forwards os.Open.
func Open(path string) (*os.File, error) { return os.Open(path) }
`,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(directory)
	for _, test := range []struct {
		kinds   string
		want    []string
		without []string
	}{
		{"result", []string{"gohawkresultfacts example.com/factkinds.Open"}, []string{"gohawklifecyclefacts", "heap "}},
		{"heap", []string{"heap edge"}, []string{"gohawkresultfacts", "no parameter is proven"}},
		{"lifecycle", []string{"gohawklifecyclefacts example.com/factkinds.Open"}, []string{"heap ", "gohawkresultfacts"}},
	} {
		var output, errorsOutput bytes.Buffer
		if err := printFacts([]string{"-func", "Open", "-kind", test.kinds, "."}, &output, &errorsOutput); err != nil {
			t.Fatalf("-kind %s: printFacts() error = %v, stderr %s", test.kinds, err, errorsOutput.String())
		}
		for _, want := range test.want {
			if !strings.Contains(output.String(), want) {
				t.Errorf("-kind %s: dump lacks %q:\n%s", test.kinds, want, output.String())
			}
		}
		for _, unwanted := range test.without {
			if strings.Contains(output.String(), unwanted) {
				t.Errorf("-kind %s: dump has %q:\n%s", test.kinds, unwanted, output.String())
			}
		}
	}
	var output, errorsOutput bytes.Buffer
	if err := printFacts([]string{"-kind", "bogus", "."}, &output, &errorsOutput); err == nil || !strings.Contains(err.Error(), "unknown fact kind") {
		t.Errorf("-kind bogus: error = %v, want unknown fact kind", err)
	}
}
