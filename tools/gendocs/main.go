// Command gendocs synchronizes analyzer metadata with the documentation site.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/docexamples"
)

const (
	generatedAnalyzersStart  = "<!-- gohawk:generated-analyzers:start -->"
	generatedAnalyzersEnd    = "<!-- gohawk:generated-analyzers:end -->"
	generatedOptionsStart    = "{/* gohawk:generated-options:start */}"
	generatedOptionsEnd      = "{/* gohawk:generated-options:end */}"
	generatedExamplesStart   = "{/* gohawk:generated-examples:start */}"
	generatedExamplesEnd     = "{/* gohawk:generated-examples:end */}"
	generatedChecksStart     = "{/* gohawk:generated-checks:start */}"
	generatedChecksEnd       = "{/* gohawk:generated-checks:end */}"
	analyzerComponentImports = "import CheckIdentity from '../../../site/src/components/CheckIdentity.astro';"
)

type manifest struct {
	Groups []group `json:"groups"`
}

type group struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Slug      string     `json:"slug"`
	Analyzers []analyzer `json:"analyzers"`
}

type analyzer struct {
	Name         string          `json:"name"`
	Summary      string          `json:"summary"`
	Path         string          `json:"path"`
	OptIn        bool            `json:"optIn"`
	Checks       []check         `json:"checks"`
	SuggestedFix bool            `json:"suggestedFix"`
	Options      []optionFlag    `json:"options"`
	Examples     docexamples.Set `json:"-"`
}

type check struct {
	ID      string           `json:"id"`
	Summary string           `json:"summary"`
	Kind    gohawk.CheckKind `json:"kind"`
	OptIn   bool             `json:"optIn"`
}

type optionFlag struct {
	Name    string `json:"name"`
	Default string `json:"default"`
	Usage   string `json:"usage"`
}

func main() {
	check := flag.Bool("check", false, "fail if generated documentation is stale")
	flag.Parse()

	root, err := repositoryRoot()
	if err == nil {
		err = synchronize(root, *check)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("could not find repository root containing go.mod")
		}
		directory = parent
	}
}
