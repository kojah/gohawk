package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPartialVetFailurePreservesOutputAndFailure(t *testing.T) {
	for _, render := range []renderMode{renderJSON, renderRich} {
		for _, payload := range []string{`{}`, `{"good":{"channelsafety":[{"posn":"good.go:1:1","message":"retained finding"}]}}`} {
			var output, errorOutput bytes.Buffer
			runtime := cliRuntime{
				output: &output, errorsOutput: &errorOutput,
				executable: func() (string, error) { return "gohawk", nil },
				execute: func(string, []string, []string) (processOutput, error) {
					return processOutput{stdout: []byte(payload), stderr: []byte("broken.go: undefined: missing\n"), exitCode: 1},
						errors.New("exit status 1")
				},
			}
			code := runViaGoVet(&analysisInvocation{arguments: []string{"./..."}, render: render}, runtime)
			if code != 1 || !strings.Contains(errorOutput.String(), "undefined: missing") {
				t.Fatalf("partial scan: code=%d, stderr=%s", code, errorOutput.String())
			}
			if render == renderJSON && !json.Valid(output.Bytes()) {
				t.Fatalf("invalid diagnostic JSON: %s", output.String())
			}
			if strings.Contains(payload, "retained finding") && !strings.Contains(output.String(), "retained finding") {
				t.Fatal("partial failure discarded available diagnostics")
			}
		}
	}
}

func TestCLIPartialPackageFailure(t *testing.T) {
	binary := buildTestBinary(t)
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/partial\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "bad", "bad.go"), "package bad\nvar Value = missing\n")
	for _, finding := range []bool{false, true} {
		source := "package good\nfunc F() {}\n"
		if finding {
			source = "package good\nfunc F(c chan int) { close(c); c <- 1 }\n"
		}
		writeTestFile(t, filepath.Join(directory, "good", "good.go"), source)
		command := exec.CommandContext(t.Context(), binary, "-json", "-enable=channelsafety", "./...")
		command.Dir = directory
		command.Env = append(os.Environ(), "GOWORK=off")
		var output, errorOutput bytes.Buffer
		command.Stdout, command.Stderr = &output, &errorOutput
		err := command.Run()
		if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
			t.Fatalf("finding=%t: err=%v, stdout=%s, stderr=%s", finding, err, output.String(), errorOutput.String())
		}
		if !json.Valid(output.Bytes()) || !strings.Contains(errorOutput.String(), "undefined: missing") {
			t.Fatalf("finding=%t: invalid output: %s / %s", finding, output.String(), errorOutput.String())
		}
		if strings.Contains(output.String(), "send follows close") != finding {
			t.Fatalf("finding=%t: diagnostics changed: %s", finding, output.String())
		}
	}
}
