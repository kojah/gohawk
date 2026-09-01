package check

import (
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Suppressed reports whether an immediately adjacent comment
// contains "gohawk:ignore analyzer" for analyzer.
func Suppressed(pass *analysis.Pass, position token.Pos, analyzer string) bool {
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
