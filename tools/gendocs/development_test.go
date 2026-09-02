package main

import (
	"strings"
	"testing"
)

func TestTraceFlagsBlockListsRegisteredFlags(t *testing.T) {
	block := traceFlagsBlock()
	for _, name := range []string{"`-gohawk-trace`", "`-gohawk-trace-source`", "`-gohawk-trace-function`", "`-gohawk-trace-file`"} {
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
	helpers, err := helpersIndexBlock(root)
	if err != nil {
		t.Fatal(err)
	}
	// A package-level function, a constructor go/doc files under its type, and
	// a method must all appear by the names the helper index cites.
	for _, want := range []string{"| `WalkStates` |", "| `NewLifecycleEvidence` |", "| `ReachingWalk.Any` |"} {
		if !strings.Contains(helpers, want) {
			t.Errorf("helper index lacks %q", want)
		}
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
