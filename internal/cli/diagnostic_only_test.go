package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRejectSourceEditFlags(t *testing.T) {
	for _, option := range []string{"-fix", "--fix", "-fix=true", "-fix=false", "-diff", "--diff=true"} {
		t.Run(option, func(t *testing.T) {
			var output, errorsOutput bytes.Buffer
			result := runCLI([]string{"gohawk", option, "./..."}, testCLIRuntime(t, &output, &errorsOutput))
			if result.exitCode != 2 || result.invocation != nil || !strings.Contains(errorsOutput.String(), "diagnostic-only") {
				t.Fatalf("result = %#v, stderr = %q", result, errorsOutput.String())
			}
		})
	}
}
