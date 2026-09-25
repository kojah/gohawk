package check

import (
	"go/ast"
	"go/token"
	"slices"

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

// KeywordEvidence cites only the keyword that opens a statement, such as the
// for of a loop, so a label names the statement without underlining its body.
func KeywordEvidence(position token.Pos, keyword, label string) analysis.RelatedInformation {
	return analysis.RelatedInformation{Pos: position, End: position + token.Pos(len(keyword)), Message: label}
}

// ReturnEvidence cites the normal return a proof reached with an obligation
// still open, labeled "returns here without <missing>". A function that falls
// off its end has no return statement, so its closing brace is cited
// instead. When one branch after start decides that the path reaches that
// return, its condition is cited too, so the reader sees which case leaks.
// It returns nothing when there is no witness.
func ReturnEvidence(pass *analysis.Pass, start ssa.Instruction, returned *ssa.Return, missing string) []analysis.RelatedInformation {
	if returned == nil {
		return nil
	}
	var evidence []analysis.RelatedInformation
	if condition, outcome, ok := decidingCondition(start, returned); ok {
		evidence = append(evidence, Evidence(pass, condition.Pos(), "when this is "+outcome))
	}
	if returned.Pos().IsValid() {
		return append(evidence, Evidence(pass, returned.Pos(), "returns here without "+missing))
	}
	if end := functionEnd(returned.Parent()); end.IsValid() {
		return append(evidence, analysis.RelatedInformation{Pos: end, End: end + 1, Message: "reaches the end of the function without " + missing})
	}
	return nil
}

// decidingCondition finds the nearest branch that every path to returned
// takes the same way: an If after start whose one successor dominates the
// return's block. A return reached from both arms, or a branch before the
// obligation began, is not cited; the explanation must hold on every path.
func decidingCondition(start ssa.Instruction, returned *ssa.Return) (ssa.Value, string, bool) {
	if start == nil || start.Block() == nil {
		return nil, "", false
	}
	target := returned.Block()
	for block := target.Idom(); block != nil; block = block.Idom() {
		if !start.Block().Dominates(block) {
			return nil, "", false
		}
		branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
		if !ok || len(block.Succs) != 2 {
			continue
		}
		// Only a condition computed after the obligation began has a source
		// position inside the if statement. A parameter or captured variable
		// used as the whole condition is positioned at its declaration, so it
		// is not cited rather than pointing at the wrong line.
		if _, computed := branch.Cond.(ssa.Instruction); !computed || branch.Cond.Pos() <= start.Pos() {
			return nil, "", false
		}
		if block == start.Block() && !slices.Contains(block.Instrs[instructionIndex(start)+1:], ssa.Instruction(branch)) {
			return nil, "", false
		}
		switch {
		case block.Succs[0].Dominates(target):
			return branch.Cond, "true", true
		case block.Succs[1].Dominates(target):
			return branch.Cond, "false", true
		}
	}
	return nil, "", false
}

// FunctionEndEvidence cites the closing brace of function, where its deferred
// calls run, or returns nothing when the function has no body.
func FunctionEndEvidence(function *ssa.Function, label string) []analysis.RelatedInformation {
	if end := functionEnd(function); end.IsValid() {
		return []analysis.RelatedInformation{{Pos: end, End: end + 1, Message: label}}
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

// instructionIndex returns the position of instruction in its block.
func instructionIndex(instruction ssa.Instruction) int {
	return slices.Index(instruction.Block().Instrs, instruction)
}
