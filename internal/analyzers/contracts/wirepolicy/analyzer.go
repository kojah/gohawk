// Package wirepolicy implements the wirepolicy gohawk analyzer.
package wirepolicy

import (
	"go/ast"
	"go/types"
	"reflect"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "wirepolicy",
		Doc:  "checks serialized structs and their composite literals",
		Run: func(pass *analysis.Pass) (any, error) {
			// Wire identity is established by either a serialization-oriented type
			// name or an actual json/toml tag. Requiring concrete evidence here keeps
			// ordinary internal structs outside both checks.
			for _, file := range pass.Files {
				if !syntax.AnalyzeFile(pass, file) {
					continue
				}
				ast.Inspect(file, func(node ast.Node) bool {
					switch typed := node.(type) {
					case *ast.CompositeLit:
						if len(typed.Elts) > 0 && wireStruct(pass.TypesInfo.TypeOf(typed)) && !allKeyed(typed.Elts) {
							check.Report(pass, check.WireKeyedLiteral, analysis.Diagnostic{
								Pos:            typed.Pos(),
								End:            typed.End(),
								Message:        "persisted or wire struct literal must use field keys",
								SuggestedFixes: keyedLiteralFix(pass.TypesInfo.TypeOf(typed), typed.Elts),
							})
						}
					case *ast.TypeSpec:
						structure, ok := typed.Type.(*ast.StructType)
						if ok {
							if ambiguousRowName(pass.TypesInfo.TypeOf(typed.Type), typed.Name.Name) {
								emitAmbiguousRowDecision(pass, typed)
							}
							if wireName(typed.Name.Name) {
								reportMissingWireTags(pass, structure)
							}
						}
					}
					return true
				})
			}
			return nil, nil
		},
	}
}

func keyedLiteralFix(value types.Type, elements []ast.Expr) []analysis.SuggestedFix {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	if !ok {
		return nil
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok || len(elements) > structure.NumFields() {
		return nil
	}
	edits := make([]analysis.TextEdit, 0, len(elements))
	for index, element := range elements {
		name := structure.Field(index).Name()
		if name == "_" {
			return nil
		}
		edits = append(edits, analysis.TextEdit{Pos: element.Pos(), NewText: []byte(name + ": ")})
	}
	return []analysis.SuggestedFix{{Message: "Add field keys", TextEdits: edits}}
}

func wireStruct(value types.Type) bool {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	if !ok {
		return false
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	if wireName(named.Obj().Name()) {
		return true
	}
	for index := range structure.NumFields() {
		tag := reflect.StructTag(structure.Tag(index))
		if tag.Get("json") != "" || tag.Get("toml") != "" {
			return true
		}
	}
	return false
}

func wireName(name string) bool {
	// Row describes display tables and compact test fixtures at least as often
	// as persistence. Treating the suffix as wire evidence produced 91 findings
	// across three unrelated UI/test models:
	// https://github.com/civitai/cli/blob/bc830b105867ae4234ddd7dd23f3f7680a2cbe3c/internal/cmd/app_status_drift_test.go#L55-L72
	// https://github.com/harshalgajjar/Reminal/blob/c4fd9e64b3b1deabaaacd5e10b9090a28792148d/cmd/reminal/main.go#L669-L710
	// https://github.com/tiny-systems/tiny/blob/950f54ee94b5065f7c7c27254e8be5bc09f81d63/internal/sessions/store.go#L43-L63
	// Concrete json/toml tags still identify an actual serialized Row.
	return strings.HasSuffix(name, "Wire") || strings.HasSuffix(name, "Envelope")
}

func ambiguousRowName(value types.Type, name string) bool {
	return strings.HasSuffix(name, "Row") && !wireStruct(value)
}

func emitAmbiguousRowDecision(pass *analysis.Pass, specification *ast.TypeSpec) {
	checkID := string(check.WireSerializationTag)
	analysisTrace.EmitIfEnabled(pass, analysisTrace.Event{
		Analyzer: "wirepolicy",
		Check:    checkID,
		Phase:    "decision",
		Reason:   "ambiguous-row-name",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      specification.Name.Pos(),
	})
}

func allKeyed(elements []ast.Expr) bool {
	for _, element := range elements {
		if _, ok := element.(*ast.KeyValueExpr); !ok {
			return false
		}
	}
	return true
}

func reportMissingWireTags(pass *analysis.Pass, structure *ast.StructType) {
	for _, field := range structure.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			if name.IsExported() && field.Tag == nil {
				check.Reportf(pass, check.WireSerializationTag, name.Pos(), "serialized field %s requires an explicit json or toml tag", name.Name)
			}
		}
	}
}
