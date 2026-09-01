// Package closedomain implements the closedomain gohawk analyzer.
package closedomain

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
)

type closedStringDomainFact struct{}

func (*closedStringDomainFact) AFact() {}

func (*closedStringDomainFact) String() string { return "closedStringDomain" }

type enumCandidate struct {
	field    *types.Var
	owner    *types.Named
	position *ast.Ident
}

type enumFlow struct {
	values       map[string]bool
	sourceFields map[*types.Var]bool
	erasedNamed  bool
	open         bool
}

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:      "closedomain",
		Doc:       "finds builtin-string fields used as closed semantic domains",
		Run:       runEnumField,
		FactTypes: []analysis.Fact{new(closedStringDomainFact)},
	}
}

func runEnumField(pass *analysis.Pass) (any, error) {
	files := enumProductionFiles(pass)
	candidates := enumCandidates(pass, files)
	locals, summaries := enumLocalFlows(pass, files)
	directValues := make(map[*types.Var]map[string]bool)
	fieldFlows := make(map[*types.Var]enumFlow)

	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.AssignStmt:
				recordEnumFieldAssignment(pass, typed, locals, summaries, fieldFlows)
			case *ast.CompositeLit:
				recordEnumFieldComposite(pass, typed, locals, summaries, fieldFlows)
			case *ast.BinaryExpr:
				recordEnumComparison(pass, typed, directValues)
			case *ast.SwitchStmt:
				recordEnumSwitch(pass, typed, directValues)
			}
			return true
		})
	}

	closed := make(map[*types.Var]bool)
	for field := range candidates {
		flow := fieldFlows[field]
		if len(directValues[field]) >= 2 || flow.erasedNamed || !flow.open && len(flow.values) >= 2 {
			closed[field] = true
		}
	}
	closeEnumTaggedUnionFields(pass, candidates, directValues, fieldFlows, closed)
	propagateClosedEnumFields(pass, candidates, fieldFlows, closed)

	for field, candidate := range candidates {
		if !closed[field] {
			continue
		}
		check.Reportf(pass, check.ClosedStringDomain, candidate.position.Pos(), "field %s uses a closed string domain; define a named string type and constants", candidate.position.Name)
		pass.ExportObjectFact(field, new(closedStringDomainFact))
	}
	return nil, nil
}

func enumProductionFiles(pass *analysis.Pass) []*ast.File {
	files := make([]*ast.File, 0, len(pass.Files))
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) || strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		files = append(files, file)
	}
	return files
}

func enumCandidates(pass *analysis.Pass, files []*ast.File) map[*types.Var]enumCandidate {
	result := make(map[*types.Var]enumCandidate)
	for _, file := range files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || enumStagingType(typeSpec.Name.Name) {
					continue
				}
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				typeName, _ := pass.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
				owner, _ := typeName.Type().(*types.Named)
				for _, field := range structure.Fields.List {
					recordEnumFieldCandidate(pass, owner, field, result)
				}
			}
		}
	}
	return result
}

func enumStagingType(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, "input") || strings.HasSuffix(name, "wire") || strings.HasPrefix(name, "raw")
}

func recordEnumFieldCandidate(pass *analysis.Pass, owner *types.Named, field *ast.Field, candidates map[*types.Var]enumCandidate) {
	if !enumBuiltinStringStorage(pass.TypesInfo.TypeOf(field.Type)) {
		return
	}
	for _, name := range field.Names {
		if !name.IsExported() || !enumFieldName(name.Name) {
			continue
		}
		variable, ok := pass.TypesInfo.Defs[name].(*types.Var)
		if ok && variable.IsField() {
			candidates[variable] = enumCandidate{field: variable, owner: owner, position: name}
		}
	}
}

func enumBuiltinStringStorage(value types.Type) bool {
	if pointer, ok := types.Unalias(value).(*types.Pointer); ok {
		value = pointer.Elem()
	}
	basic, ok := types.Unalias(value).(*types.Basic)
	return ok && basic.Kind() == types.String
}

func enumFieldName(name string) bool {
	switch strings.ToLower(name) {
	case "action", "adapter", "code", "coverage", "granularity", "kind", "level", "mode", "outcome", "phase", "plugin", "provider", "reason", "requirement", "resultsource", "role", "severity", "state", "status", "trigger", "type":
		return true
	default:
		return false
	}
}
