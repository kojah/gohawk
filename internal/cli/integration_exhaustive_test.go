package cli

import (
	"os"
	"testing"
)

func TestCLIIntegrationExhaustive(t *testing.T) {
	if os.Getenv("GOHAWK_EXHAUSTIVE_CLI") == "" {
		t.Skip("set GOHAWK_EXHAUSTIVE_CLI=1 to run the redundant subprocess matrix")
	}
	binary := buildTestBinary(t)
	module := writeTestModule(t)

	runExhaustiveSelectionScenarios(t, binary, module)
	runExhaustiveExecutionScenarios(t, binary, module)
}
