package check

import (
	"errors"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestAnalyzerReportFilterRestoresPassAndPreservesResult(t *testing.T) {
	failure := errors.New("analysis failed")
	var got []string
	calls := 0
	analyzer := &analysis.Analyzer{Name: "sample", Run: func(pass *analysis.Pass) (any, error) {
		calls++
		pass.Report(analysis.Diagnostic{Category: "sample/disabled", Message: "disabled"})
		pass.Report(analysis.Diagnostic{Category: "sample/enabled", Message: "enabled"})
		return "evidence", failure
	}}
	filtered := FilterAnalyzerReports(analyzer, map[string]bool{"sample/disabled": true}, 2)
	pass := &analysis.Pass{Report: func(d analysis.Diagnostic) { got = append(got, d.Message) }}
	result, err := filtered.Run(pass)
	if filtered == analyzer || result != "evidence" || !errors.Is(err, failure) || calls != 1 || len(got) != 1 || got[0] != "enabled" {
		t.Fatalf("result=%v error=%v calls=%d reports=%v", result, err, calls, got)
	}
	pass.Report(analysis.Diagnostic{Category: "sample/disabled", Message: "restored"})
	if len(got) != 2 || got[1] != "restored" {
		t.Fatal("report function was not restored")
	}
	if _, err := analyzer.Run(pass); !errors.Is(err, failure) || calls != 2 || len(got) != 4 {
		t.Fatal("original analyzer was changed")
	}
}

func TestAnalyzerReportFilterSkipsAllDisabled(t *testing.T) {
	analyzer := &analysis.Analyzer{Name: "sample", Run: func(*analysis.Pass) (any, error) { t.Fatal("disabled analyzer ran"); return nil, nil }}
	filtered := FilterAnalyzerReports(analyzer, map[string]bool{"sample/only": true}, 1)
	if result, err := filtered.Run(&analysis.Pass{}); result != nil || err != nil {
		t.Fatalf("result=%v error=%v", result, err)
	}
}
