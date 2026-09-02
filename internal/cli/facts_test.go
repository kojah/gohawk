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
`)
	t.Chdir(directory)
	var output, errorsOutput bytes.Buffer
	if err := printFacts([]string{"-all", "."}, &output, &errorsOutput); err != nil {
		t.Fatalf("printFacts() error = %v, stderr %s", err, errorsOutput.String())
	}
	text := output.String()
	for _, want := range []string{"CloseFile (exported here", "0 file: Closed", "MaybeClose (summarized here)", "no parameter is proven"} {
		if !strings.Contains(text, want) {
			t.Errorf("facts dump lacks %q:\n%s", want, text)
		}
	}
	output.Reset()
	if err := printFacts([]string{"."}, &output, &errorsOutput); err != nil {
		t.Fatalf("printFacts() error = %v", err)
	}
	if strings.Contains(output.String(), "MaybeClose") {
		t.Errorf("facts dump without -all printed an empty summary:\n%s", output.String())
	}
}
