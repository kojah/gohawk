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
// assignments. It stops at exact, statically resolved decoder calls and fields
// reachable through the pointed-to aggregate; dynamic decoder plumbing remains
// unknown.

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
			//
			// Follow only JSON-populatable aggregate edges. Minio's HealDriveInfo is
			// reached through slices and anonymous Before/After structs from the exact
			// HealTaskStatus passed to Unmarshal; the server can therefore add states
			// that local comparisons do not enumerate:
			// https://github.com/minio/madmin-go/blob/ef04ea3969c2177b22e13e9e61dfc4ddeccf3feb/heal-commands.go#L103-L165
			// https://github.com/minio/madmin-go/blob/ef04ea3969c2177b22e13e9e61dfc4ddeccf3feb/heal-commands.go#L310-L319
			owner := enumPointedNamed(pass.TypesInfo.TypeOf(call.Args[1]))
			if owner == nil {
				return true
			}
			for field, candidate := range candidates {
				if candidate.owner != nil && jsonPopulatesField(owner, field) {
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
	return jsonAggregateReachesField(owner, field, make(map[types.Type]bool))
}

func jsonAggregateReachesField(current types.Type, target *types.Var, seen map[types.Type]bool) bool {
	current = types.Unalias(current)
	if seen[current] {
		return false
	}
	seen[current] = true

	switch typed := current.(type) {
	case *types.Named:
		return jsonAggregateReachesField(typed.Underlying(), target, seen)
	case *types.Pointer:
		return jsonAggregateReachesField(typed.Elem(), target, seen)
	case *types.Array:
		return jsonAggregateReachesField(typed.Elem(), target, seen)
	case *types.Slice:
		return jsonAggregateReachesField(typed.Elem(), target, seen)
	case *types.Struct:
		for index := range typed.NumFields() {
			field := typed.Field(index)
			if !field.Exported() || reflect.StructTag(typed.Tag(index)).Get("json") == "-" {
				continue
			}
			if field == target || jsonAggregateReachesField(field.Type(), target, seen) {
				return true
			}
		}
	}
	return false
}

func traceExternalDeserialization(pass *analysis.Pass, candidate enumCandidate, proof token.Pos) {
	analysisTrace.For(pass, "closedomain", string(check.ClosedStringDomain), proof).Evidence(analysisTrace.Step{
		Reason:  "external-deserialization",
		Outcome: analysisTrace.OutcomeAccepted,
		Pos:     proof,
		Details: map[string]string{"field": candidate.position.Name},
	})
}
