package closedomain

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

var jsonUnmarshal = syntax.PackageFunction("encoding/json", "Unmarshal")

// This file recognizes external deserialization that bypasses ordinary field
// assignments. It stops at exact, statically resolved decoder calls and the
// directly pointed-to struct; dynamic decoder plumbing remains unknown.

func enumExternallyDeserializedFields(
	pass *analysis.Pass,
	files []*ast.File,
	candidates map[*types.Var]enumCandidate,
) map[*types.Var]token.Pos {
	result := make(map[*types.Var]token.Pos)
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || !syntax.IsCallTo(pass, call, jsonUnmarshal) {
				return true
			}
			// encoding/json writes exported fields through reflection, so source-level
			// assignments cannot bound values received this way. A tag alone proves
			// nothing; the exact Unmarshal identity and enclosing pointer provide the
			// boundary evidence. This occurs in WorkOS's API error decoder:
			// https://github.com/workos/workos-go/blob/be30f8d82e238934a68fdb0c0193c72351d130bd/client.go#L220-L248
			owner := enumPointedNamed(pass.TypesInfo.TypeOf(call.Args[1]))
			if owner == nil {
				return true
			}
			for field, candidate := range candidates {
				if candidate.owner != nil && types.Identical(candidate.owner, owner) && jsonPopulatesField(owner, field) {
					result[field] = call.Pos()
				}
			}
			return true
		})
	}
	return result
}

func enumPointedNamed(value types.Type) *types.Named {
	pointer, ok := types.Unalias(value).(*types.Pointer)
	if !ok {
		return nil
	}
	named, _ := types.Unalias(pointer.Elem()).(*types.Named)
	return named
}

func jsonPopulatesField(owner *types.Named, field *types.Var) bool {
	structure, ok := owner.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for index := range structure.NumFields() {
		if structure.Field(index) != field {
			continue
		}
		return reflect.StructTag(structure.Tag(index)).Get("json") != "-"
	}
	return false
}

func traceExternalDeserialization(pass *analysis.Pass, candidate enumCandidate, proof token.Pos) {
	if !analysisTrace.Enabled("closedomain", string(check.ClosedStringDomain)) {
		return
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "closedomain",
		Check:    string(check.ClosedStringDomain),
		Phase:    "evidence",
		Reason:   "external-deserialization",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      proof,
		Details:  map[string]string{"field": candidate.position.Name},
	})
}
