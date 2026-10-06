package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
)

// Delegated execution runs go vet with this binary as its analysis tool.
// It preserves process failures even when some packages emit valid findings;
// presentation and JSON normalization consume the resulting command output.

type processOutput struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type processExecutor func(name string, arguments, environment []string) (processOutput, error)

func executeProcess(name string, arguments, environment []string) (processOutput, error) {
	command := exec.CommandContext(context.Background(), name, arguments...)
	command.Env = append(os.Environ(), environment...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := processOutput{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		result.exitCode = exitError.ExitCode()
		return result, err
	}
	result.exitCode = -1
	return result, err
}

// runViaGoVet analyzes the requested packages by invoking `go vet` with this
// binary as its tool. go vet writes analysis results as JSON to stdout and
// build or load failures to stderr, so a stdout that is not valid JSON means
// the packages did not compile and its stderr is the real error.
func runViaGoVet(invocation *analysisInvocation, runtime cliRuntime) int {
	self, err := runtime.executable()
	if err != nil {
		writeFormattedf(runtime.errorsOutput, "gohawk: locate executable: %v\n", err)
		return 1
	}
	arguments := append([]string{"vet", "-vettool=" + self, "-json"}, invocation.arguments...)
	result, execErr := runtime.execute("go", arguments, nil)
	merged, mergeErr := mergeVetOutput(result.stdout)
	// go vet prints nothing to stdout when the packages fail to build, so a
	// failing exit with no analysis output means stderr holds the real error.
	if mergeErr != nil || execErr != nil && len(bytes.TrimSpace(result.stdout)) == 0 {
		_, _ = runtime.errorsOutput.Write(result.stderr)
		if execErr != nil && result.exitCode > 0 {
			return result.exitCode
		}
		if execErr != nil {
			writeFormattedf(runtime.errorsOutput, "gohawk: run go vet: %v\n", execErr)
		}
		return 1
	}
	if len(result.stderr) > 0 {
		_, _ = runtime.errorsOutput.Write(result.stderr)
	}
	var code int
	switch invocation.render {
	case renderJSON:
		_, _ = runtime.output.Write(merged)
		code = jsonDiagnosticExitCode(merged)
	default:
		code = renderDelegatedDiagnostics(merged, invocation.contextLines, runtime.output)
	}
	// A successful package may emit valid JSON while another fails to load.
	// Render the available findings, but do not turn that partial run into a
	// success. In JSON mode unitchecker reports findings as data, not a failing
	// process status; an execution failure therefore takes precedence.
	if execErr != nil {
		return max(1, result.exitCode)
	}
	return code
}
