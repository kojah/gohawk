package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintSSA(t *testing.T) {
	directory := t.TempDir()
	writeFile := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("go.mod", "module example.com/ssadump\n\ngo 1.25\n")
	writeFile("main.go", `package main

import "os"

func run(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return nil
}

func main() { _ = run("x") }
`)
	t.Chdir(directory)

	var output, errorsOutput bytes.Buffer
	if err := printSSA([]string{"-func", "run", "."}, &output, &errorsOutput); err != nil {
		t.Fatalf("printSSA() error = %v, stderr %s", err, errorsOutput.String())
	}
	text := output.String()
	for _, want := range []string{"func run(path string) error:", "defer", "make closure run$1", "func run$1():", "Close"} {
		if !strings.Contains(text, want) {
			t.Errorf("SSA dump lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "func main():") {
		t.Errorf("SSA dump printed unselected function main:\n%s", text)
	}
	if err := printSSA([]string{"-func", "missing", "."}, &output, &errorsOutput); err == nil {
		t.Error("printSSA() with no matching function succeeded, want error")
	}
}
