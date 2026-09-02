package globalstate

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Embedded byte-slice evidence recognizes compiler-initialized storage and
// accepts it only when the ordinary collection proof resolves every use as a
// read. Exported bindings, mutation, and uncertain escapes remain reportable.

func embeddedByteSliceReadOnly(
	pass *analysis.Pass,
	name *ast.Ident,
	object types.Object,
	declaration *ast.GenDecl,
	specification *ast.ValueSpec,
	index int,
	usage globalStateUsage,
) (recognized, readOnly bool) {
	if !embeddedByteSlice(declaration, specification, index, object) {
		return false, false
	}
	if name.IsExported() {
		return true, false
	}
	// The compiler provides unique backing storage for an embedded byte slice.
	// Require the same complete package-use proof as literal collections. Dead
	// Man's Switch serves embedded API documentation through ResponseWriter.Write:
	// https://github.com/circa10a/dead-mans-switch/blob/9be8307d6709f559ff97004dedfbb6e7ea22f350/internal/server/server.go#L38-L45
	readOnly = collectionObjectReadOnly(pass, object, usage, map[types.Object]bool{object: true})
	if readOnly {
		emitEmbeddedByteSliceReadOnly(pass, name)
	}
	return true, readOnly
}

func embeddedByteSlice(declaration *ast.GenDecl, specification *ast.ValueSpec, index int, object types.Object) bool {
	if declaration == nil || specification == nil || object == nil || len(declaration.Specs) != 1 || len(specification.Names) != 1 ||
		index != 0 || len(specification.Values) != 0 || !hasEmbedDirective(declaration.Doc) {
		return false
	}
	slice, ok := object.Type().Underlying().(*types.Slice)
	if !ok {
		return false
	}
	element, ok := slice.Elem().Underlying().(*types.Basic)
	return ok && element.Kind() == types.Uint8
}

func hasEmbedDirective(comments *ast.CommentGroup) bool {
	if comments == nil {
		return false
	}
	for _, comment := range comments.List {
		if strings.HasPrefix(comment.Text, "//go:embed ") || strings.HasPrefix(comment.Text, "//go:embed\t") {
			return true
		}
	}
	return false
}

func emitEmbeddedByteSliceReadOnly(pass *analysis.Pass, name *ast.Ident) {
	traceAcceptedGlobal(pass, name, "embedded-byte-slice-read-only")
}
