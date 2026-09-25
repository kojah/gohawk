package check

import (
	"go/ast"
	"go/token"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Evidence describes one location that supports a diagnostic, such as the
// return that leaks a resource, spanning the whole source node there. The
// terminal output draws it as a labeled span and editors list it as related
// information.
func Evidence(pass *analysis.Pass, position token.Pos, label string) analysis.RelatedInformation {
	source := syntax.SourceRange(pass, position)
	return analysis.RelatedInformation{Pos: source.Pos(), End: source.End(), Message: label}
}

// ReturnEvidence cites the normal return a proof reached with an obligation
// still open, labeled "returns here without <missing>". A function that falls
// off its end has no return statement, so its closing brace is cited
// instead. It returns nothing when there is no witness.
func ReturnEvidence(pass *analysis.Pass, returned *ssa.Return, missing string) []analysis.RelatedInformation {
	if returned == nil {
		return nil
	}
	if returned.Pos().IsValid() {
		return []analysis.RelatedInformation{Evidence(pass, returned.Pos(), "returns here without "+missing)}
	}
	if end := functionEnd(returned.Parent()); end.IsValid() {
		return []analysis.RelatedInformation{{Pos: end, End: end + 1, Message: "reaches the end of the function without " + missing}}
	}
	return nil
}

func functionEnd(function *ssa.Function) token.Pos {
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		if syntax.Body != nil {
			return syntax.Body.Rbrace
		}
	case *ast.FuncLit:
		return syntax.Body.Rbrace
	}
	return token.NoPos
}
