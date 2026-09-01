//go:build exhaustive

package cli

import (
	"testing"
)

func TestCLIIntegrationExhaustive(t *testing.T) {
	binary := buildTestBinary(t)
	module := writeTestModule(t)

	runExhaustiveSelectionScenarios(t, binary, module)
	runExhaustiveExecutionScenarios(t, binary, module)
}
