package main

import (
	"strings"
	"testing"
)

func TestTraceFlagsBlockListsRegisteredFlags(t *testing.T) {
	block := traceFlagsBlock()
	for _, name := range []string{"`-gohawk-trace`", "`-gohawk-trace-candidate`", "`-gohawk-trace-file`"} {
		if !strings.Contains(block, name) {
			t.Errorf("trace flags block lacks %s:\n%s", name, block)
		}
	}
}

func TestDevelopmentBlocksRenderFromSource(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := parsePackageDoc(root, "internal/ssaflow")
	if err != nil {
		t.Fatal(err)
	}
	helpers, err := helperReference(root, "internal/ssaflow", pkg)
	if err != nil {
		t.Fatal(err)
	}
	// Functions, constructors, methods, types, and constants are all searchable.
	for _, want := range []string{"## WalkStates", "## NewReachingWalk", "## ReachingWalk.Any", "## TransparentValueForm", "```go", "[Source]("} {
		if !strings.Contains(helpers, want) {
			t.Errorf("helper index lacks %q", want)
		}
	}
	example, err := ssaExampleBlock(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func CopyHeader(", "defer (*os.File).Close", "make closure CopyHeader$1"} {
		if !strings.Contains(example, want) {
			t.Errorf("ssa example block lacks %q:\n%s", want, example)
		}
	}
	if strings.Contains(example, root) {
		t.Errorf("ssa example block leaks the repository root:\n%s", example)
	}
	fields, err := factFieldsBlock(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"```go", "type Fact struct", "Retained", "ParameterMask"} {
		if !strings.Contains(fields, want) {
			t.Errorf("fact fields block lacks %q:\n%s", want, fields)
		}
	}
}
