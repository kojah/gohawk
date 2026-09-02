// Development documentation blocks are generated from the code they describe,
// so the helper index, the Fact declaration, and the trace flags cannot drift
// from the source. The curated prose around each block stays hand-written;
// only the inventory between the markers is regenerated, and the -check mode
// fails when a committed page no longer matches the code.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/trace"
)

const (
	generatedHelpersStart    = "<!-- gohawk:generated-helpers:start -->"
	generatedHelpersEnd      = "<!-- gohawk:generated-helpers:end -->"
	generatedFactFieldsStart = "<!-- gohawk:generated-fact-fields:start -->"
	generatedFactFieldsEnd   = "<!-- gohawk:generated-fact-fields:end -->"
	generatedTraceFlagsStart = "<!-- gohawk:generated-trace-flags:start -->"
	generatedTraceFlagsEnd   = "<!-- gohawk:generated-trace-flags:end -->"
	modulePath               = "github.com/kojah/gohawk"
)

// helperPackages are the packages whose exported surface the shared-helpers
// index inventories.
var helperPackages = []string{"internal/ssaflow", "internal/passes/lifecyclefacts"}

// developmentBlock is one generated region inside a hand-written page under
// docs/development.
type developmentBlock struct {
	page   string
	start  string
	end    string
	render func(root string) (string, error)
}

var developmentBlocks = []developmentBlock{
	{page: "shared-helpers.md", start: generatedHelpersStart, end: generatedHelpersEnd, render: helpersIndexBlock},
	{page: "fact-model.md", start: generatedFactFieldsStart, end: generatedFactFieldsEnd, render: factFieldsBlock},
	{
		page: "debugging-reference.md", start: generatedTraceFlagsStart, end: generatedTraceFlagsEnd,
		render: func(string) (string, error) { return traceFlagsBlock(), nil },
	},
}

// synchronizeDevelopmentDocs regenerates every development block and records
// the resulting page contents in updates for the shared write-or-check step.
func synchronizeDevelopmentDocs(root string, updates map[string][]byte) error {
	for _, block := range developmentBlocks {
		page := filepath.Join(root, "docs", "development", block.page)
		contents, err := os.ReadFile(page)
		if err != nil {
			return err
		}
		body, err := block.render(root)
		if err != nil {
			return fmt.Errorf("render %s: %w", relativePath(root, page), err)
		}
		contents, err = replaceGeneratedBlock(contents, block.start, block.end, body)
		if err != nil {
			return fmt.Errorf("update %s: %w", relativePath(root, page), err)
		}
		updates[page] = contents
	}
	return nil
}

// helpersIndexBlock lists every exported function and method of the helper
// packages with the synopsis of its doc comment, sorted by name. Constructors
// that go/doc files under their result type are listed by their own name, so
// the index matches the package-level functions the architecture test
// requires the page to cover.
func helpersIndexBlock(root string) (string, error) {
	var rows []string
	for _, directory := range helperPackages {
		pkg, err := parsePackageDoc(root, directory)
		if err != nil {
			return "", err
		}
		rows = append(rows, helperRows(pkg)...)
	}
	slices.Sort(rows)
	return "| helper | package | what it does |\n|---|---|---|\n" + strings.Join(rows, "\n"), nil
}

func parsePackageDoc(root, directory string) (*doc.Package, error) {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(directory), "*.go"))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Go source in %s", directory)
	}
	return doc.NewFromFiles(fset, files, modulePath+"/"+directory)
}

func helperRows(pkg *doc.Package) []string {
	var rows []string
	row := func(name, synopsis string) {
		rows = append(rows, "| `"+name+"` | "+pkg.Name+" | "+markdownTableCell(synopsis)+" |")
	}
	for _, function := range pkg.Funcs {
		row(function.Name, pkg.Synopsis(function.Doc))
	}
	for _, typ := range pkg.Types {
		for _, constructor := range typ.Funcs {
			row(constructor.Name, pkg.Synopsis(constructor.Doc))
		}
		for _, method := range typ.Methods {
			row(typ.Name+"."+method.Name, pkg.Synopsis(method.Doc))
		}
	}
	return rows
}

// factFieldsBlock prints the Fact declaration with its field comments exactly
// as the source declares it, so the documented mask list is always the real
// one.
func factFieldsBlock(root string) (string, error) {
	fset := token.NewFileSet()
	path := filepath.Join(root, "internal", "passes", "lifecyclefacts", "fact.go")
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE || !declaresType(general, "Fact") {
			continue
		}
		var buffer bytes.Buffer
		node := &printer.CommentedNode{Node: general, Comments: file.Comments}
		if err := printer.Fprint(&buffer, fset, node); err != nil {
			return "", err
		}
		return "```go\n" + buffer.String() + "\n```", nil
	}
	return "", fmt.Errorf("%s does not declare type Fact", relativePath(root, path))
}

func declaresType(general *ast.GenDecl, name string) bool {
	for _, spec := range general.Specs {
		if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.Name == name {
			return true
		}
	}
	return false
}

// traceFlagsBlock lists the evidence-tracing flags from the same registration
// the analyzers use, so the documented names and usage text are the real ones.
func traceFlagsBlock() string {
	flags := flag.NewFlagSet("gohawk", flag.ContinueOnError)
	trace.RegisterFlags(flags)
	rows := []string{"| flag | effect |", "|---|---|"}
	flags.VisitAll(func(item *flag.Flag) {
		rows = append(rows, "| `-"+item.Name+"` | "+markdownTableCell(item.Usage)+" |")
	})
	return strings.Join(rows, "\n")
}
