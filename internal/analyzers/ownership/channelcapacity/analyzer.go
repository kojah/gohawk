// Package channelcapacity implements the channelcapacity gohawk analyzer.
package channelcapacity

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	maximum := int64(1)
	analyzer := &analysis.Analyzer{Name: "channelcapacity", Doc: "checks that large channel buffers document their bound"}
	analyzer.Flags.Int64Var(&maximum, "max-unexplained-capacity", 1, "largest channel capacity allowed without a rationale; negative disables the check")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			if !analysisutil.AnalyzeFile(pass, file) {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					checkChannelCapacity(pass, file, call, maximum)
				}
				return true
			})
		}
		return nil, nil
	}
	return analyzer
}

func checkChannelCapacity(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, maximum int64) {
	if maximum < 0 {
		return
	}
	// Capacity rationale is an operational ownership policy for production
	// queues. A channel created in a test file is fixture synchronization with a
	// test-scoped lifetime. ccLoad uses fixed buffers to collect known concurrent
	// results:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/admin_codex_auth_test.go#L3768-L3784
	if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
		return
	}
	builtin, ok := call.Fun.(*ast.Ident)
	if !ok || builtin.Name != "make" || len(call.Args) < 2 {
		return
	}
	if _, ok := pass.TypesInfo.TypeOf(call).Underlying().(*types.Chan); !ok {
		return
	}
	value := pass.TypesInfo.Types[call.Args[1]].Value
	if value == nil {
		return
	}
	capacity, exact := constant.Int64Val(value)
	if !exact || capacity <= maximum || channelRationale(pass, file, call.Pos()) {
		return
	}
	check.Reportf(pass, check.ChannelCapacityRationale, call.Args[1].Pos(), "channel capacity %d requires a bounded rationale comment", capacity)
}

func channelRationale(pass *analysis.Pass, file *ast.File, position token.Pos) bool {
	line := pass.Fset.Position(position).Line
	for _, group := range file.Comments {
		commentLine := pass.Fset.Position(group.Pos()).Line
		if commentLine != line && commentLine != line-1 {
			continue
		}
		text := strings.ToLower(group.Text())
		if strings.Contains(text, "bounded:") || strings.Contains(text, "capacity:") {
			return true
		}
	}
	return false
}
