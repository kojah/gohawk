package check

import (
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestBufferReports(t *testing.T) {
	prerequisite := &analysis.Analyzer{Name: "prerequisite"}
	var reported []analysis.Diagnostic
	pass := &analysis.Pass{
		ResultOf: map[*analysis.Analyzer]any{prerequisite: "evidence"},
		Report:   func(diagnostic analysis.Diagnostic) { reported = append(reported, diagnostic) },
	}
	buffered, commit := BufferReports(pass)
	if buffered == pass || buffered.ResultOf[prerequisite] != "evidence" {
		t.Fatal("buffer must copy the pass while preserving prerequisite evidence")
	}
	buffered.Report(analysis.Diagnostic{Message: "first", Category: "test/check"})
	buffered.Report(analysis.Diagnostic{Message: "second", Category: "test/check"})
	if len(reported) != 0 {
		t.Fatal("uncommitted reports leaked")
	}
	commit()
	commit()
	if len(reported) != 2 || reported[0].Message != "first" || reported[1].Message != "second" || reported[0].Category != "test/check" {
		t.Fatalf("commit lost, changed, reordered, or duplicated reports: %+v", reported)
	}
	aborted, _ := BufferReports(pass)
	aborted.Report(analysis.Diagnostic{Message: "abandoned"})
	if len(reported) != 2 {
		t.Fatal("abandoned buffer leaked a report")
	}
	pass.Report(analysis.Diagnostic{Message: "original"})
	if len(reported) != 3 {
		t.Fatal("buffer modified the original pass")
	}
}
