package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	fileCodeTrigger        = 100
	fileGapLimit           = 150
	functionCodeTrigger    = 45
	functionBranchTrigger  = 8
	functionNestingTrigger = 4
	functionGapLimit       = 40
)

type commentaryStats struct {
	codeLines      int
	rationaleLines int
	branches       int
	maxNesting     int
	maxGap         int
}

func TestAnalyzerCommentaryCoverage(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate commentary architecture test")
	}
	internalRoot := filepath.Dir(filepath.Dir(currentFile))
	findings := make(map[string]commentaryStats)
	for _, directory := range []string{"analyzers", "analysispasses"} {
		collectCommentaryFindings(t, internalRoot, directory, findings)
	}
	for key, stats := range findings {
		t.Errorf(
			"%s has an unexplained span of %d code lines (code=%d branches=%d nesting=%d); add model-level rationale",
			key,
			stats.maxGap,
			stats.codeLines,
			stats.branches,
			stats.maxNesting,
		)
	}
}

func collectCommentaryFindings(t *testing.T, internalRoot, directory string, findings map[string]commentaryStats) {
	t.Helper()
	root := filepath.Join(internalRoot, directory)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return inspectCommentaryFile(internalRoot, path, findings)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func inspectCommentaryFile(internalRoot, path string, findings map[string]commentaryStats) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.ParseComments)
	if err != nil {
		return err
	}
	if ast.IsGenerated(file) {
		return nil
	}
	lines := sourceLines(source)
	comments, rationale := commentLines(fset, file)
	relative, err := filepath.Rel(internalRoot, path)
	if err != nil {
		return err
	}
	relative = filepath.ToSlash(relative)
	start := firstImplementationLine(fset, file)
	fileStats := spanStats(lines, comments, rationale, start, len(lines))
	if fileStats.codeLines >= fileCodeTrigger && (fileStats.rationaleLines == 0 || fileStats.maxGap > fileGapLimit) {
		findings[relative] = fileStats
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		stats := spanStats(lines, comments, rationale, fset.Position(function.Body.Lbrace).Line, fset.Position(function.Body.Rbrace).Line)
		complexity := functionComplexity(function.Body)
		stats.branches, stats.maxNesting = complexity.branches, complexity.maxNesting
		if (stats.codeLines >= functionCodeTrigger || stats.branches >= functionBranchTrigger || stats.maxNesting >= functionNestingTrigger) &&
			stats.maxGap > functionGapLimit {
			findings[relative+":"+function.Name.Name] = stats
		}
	}
	return nil
}

func sourceLines(source []byte) []string {
	return strings.Split(string(source), "\n")
}

func commentLines(fset *token.FileSet, file *ast.File) (map[int]bool, map[int]bool) {
	comments := make(map[int]bool)
	rationale := make(map[int]bool)
	for _, group := range file.Comments {
		start := fset.Position(group.Pos()).Line
		end := fset.Position(group.End()).Line
		for line := start; line <= end; line++ {
			comments[line] = true
		}
		if rationaleComment(group) {
			for line := start; line <= end; line++ {
				rationale[line] = true
			}
		}
	}
	return comments, rationale
}

func rationaleComment(group *ast.CommentGroup) bool {
	text := strings.TrimSpace(group.Text())
	if text == "" || strings.HasPrefix(text, "Analyzer returns ") || strings.HasPrefix(text, "Package ") {
		return false
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "nolint") || strings.HasPrefix(lower, "go:") || strings.HasPrefix(lower, "gohawk:") ||
		strings.HasPrefix(lower, "todo") || strings.HasPrefix(lower, "copyright") {
		return false
	}
	return !strings.HasPrefix(text, "http://") && !strings.HasPrefix(text, "https://")
}

func firstImplementationLine(fset *token.FileSet, file *ast.File) int {
	for _, declaration := range file.Decls {
		if generated, ok := declaration.(*ast.GenDecl); ok && generated.Tok == token.IMPORT {
			continue
		}
		return fset.Position(declaration.Pos()).Line
	}
	return fset.Position(file.Package).Line
}

func spanStats(lines []string, comments, rationale map[int]bool, start, end int) commentaryStats {
	stats := commentaryStats{}
	currentGap := 0
	start = max(1, start)
	end = min(end, len(lines))
	for line := start; line <= end; line++ {
		if rationale[line] {
			stats.rationaleLines++
			currentGap = 0
			continue
		}
		if comments[line] || strings.TrimSpace(lines[line-1]) == "" {
			continue
		}
		stats.codeLines++
		currentGap++
		stats.maxGap = max(stats.maxGap, currentGap)
	}
	return stats
}

type complexityStats struct {
	branches   int
	maxNesting int
}

type complexityVisitor struct {
	stats *complexityStats
	depth int
}

func functionComplexity(body *ast.BlockStmt) complexityStats {
	stats := complexityStats{}
	ast.Walk(complexityVisitor{stats: &stats}, body)
	return stats
}

func (visitor complexityVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	if branchingNode(node) {
		visitor.stats.branches++
		visitor.depth++
		visitor.stats.maxNesting = max(visitor.stats.maxNesting, visitor.depth)
	}
	return visitor
}

func branchingNode(node ast.Node) bool {
	switch node.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return true
	default:
		return false
	}
}
