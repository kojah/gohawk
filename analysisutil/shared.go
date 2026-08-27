package analysisutil

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// BuiltinClose names Go's channel-closing builtin.
const BuiltinClose = "close"

// GeneratedFile reports whether file carries Go's generated-file marker.
func GeneratedFile(file *ast.File) bool {
	return ast.IsGenerated(file)
}

// AnalyzeFile reports whether file is the canonical copy to analyze. Test
// variants contain production files a second time; only their test files are
// canonical because production files are analyzed with the ordinary package.
func AnalyzeFile(pass *analysis.Pass, file *ast.File) bool {
	if GeneratedFile(file) {
		return false
	}
	if !testVariant(pass) {
		return true
	}
	return strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
}

func testVariant(pass *analysis.Pass) bool {
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			return true
		}
	}
	return false
}

// DiagnosticSuppressed reports whether an immediately adjacent comment
// contains "gohawk:ignore analyzer" for analyzer.
func DiagnosticSuppressed(pass *analysis.Pass, position token.Pos, analyzer string) bool {
	line := pass.Fset.Position(position).Line
	for _, file := range pass.Files {
		if position < file.Pos() || position > file.End() {
			continue
		}
		for _, group := range file.Comments {
			first := pass.Fset.Position(group.Pos()).Line
			last := pass.Fset.Position(group.End()).Line
			if last != line-1 && (line < first || line > last) {
				continue
			}
			for _, comment := range group.List {
				if suppressionComment(comment.Text, analyzer) {
					return true
				}
			}
		}
	}
	return false
}

func suppressionComment(comment, analyzer string) bool {
	text := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(comment), "//"), "*/"))
	text = strings.TrimSpace(strings.TrimPrefix(text, "/*"))
	fields := strings.Fields(text)
	return len(fields) >= 2 && fields[0] == "gohawk:ignore" && fields[1] == analyzer
}

// FunctionSymbol identifies one package-level Go function.
type FunctionSymbol struct {
	Package string
	Name    string
}

// IsPackageCall reports whether call statically names symbol.
func IsPackageCall(pass *analysis.Pass, call *ast.CallExpr, symbol FunctionSymbol) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != symbol.Name {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	imported, ok := pass.TypesInfo.Uses[identifier].(*types.PkgName)
	return ok && imported.Imported().Path() == symbol.Package
}

// IsErrorType reports whether value implements Go's predeclared error interface.
func IsErrorType(value types.Type) bool {
	if value == nil {
		return false
	}
	errorType, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	return ok && types.Implements(value, errorType)
}

// IsStringType reports whether value has string as its underlying type.
func IsStringType(value types.Type) bool {
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsString != 0
}

// NamedType reports whether value names packagePath.name, allowing one pointer layer.
func NamedType(value types.Type, packagePath, name string) bool {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == packagePath && named.Obj().Name() == name
}
