package cli

import "github.com/kojah/gohawk/internal/reporting/vetjson"

// Exit status for a delegated run follows the decoded document: analyzer
// errors take precedence over findings, and malformed output is a failure.

func jsonDiagnosticExitCode(data []byte) int {
	diagnostics, analysisErrors, err := vetjson.Decode(data)
	if err != nil {
		return 1
	}
	return diagnosticExitCode(diagnostics, analysisErrors)
}

func diagnosticExitCode(diagnostics []vetjson.Diagnostic, analysisErrors []string) int {
	if len(analysisErrors) > 0 {
		return 1
	}
	if len(diagnostics) > 0 {
		return 3
	}
	return 0
}
