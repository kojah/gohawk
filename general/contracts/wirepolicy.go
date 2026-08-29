package contracts

import (
	"go/ast"
	"go/types"
	"reflect"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func wirePolicyAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "wirepolicy",
		Doc:  "checks serialized structs and their composite literals",
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				if !analysisutil.AnalyzeFile(pass, file) {
					continue
				}
				ast.Inspect(file, func(node ast.Node) bool {
					switch typed := node.(type) {
					case *ast.CompositeLit:
						if len(typed.Elts) > 0 && wireStruct(pass.TypesInfo.TypeOf(typed)) && !allKeyed(typed.Elts) {
							report(pass, checkWireKeyedLiteral, analysis.Diagnostic{
								Pos:            typed.Pos(),
								End:            typed.End(),
								Message:        "persisted or wire struct literal must use field keys",
								SuggestedFixes: keyedLiteralFix(pass.TypesInfo.TypeOf(typed), typed.Elts),
							})
						}
					case *ast.TypeSpec:
						structure, ok := typed.Type.(*ast.StructType)
						if ok && wireName(typed.Name.Name) {
							reportMissingWireTags(pass, structure)
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
	return strings.HasSuffix(name, "Wire") || strings.HasSuffix(name, "Row") || strings.HasSuffix(name, "Envelope")
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
				reportf(pass, checkWireSerializationTag, name.Pos(), "serialized field %s requires an explicit json or toml tag", name.Name)
			}
		}
	}
}
