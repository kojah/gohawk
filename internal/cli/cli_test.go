package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	gohawk "github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
)

func TestPrintAnalyzerList(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		contains  []string
		excludes  []string
		wantError bool
	}{
		{
			name:     "all",
			contains: []string{"ANALYZER", "TIER", "GROUP", "determinism", "extended", "oncepolicy", "core", "reliability", "reliability", "core runs by default"},
			excludes: []string{"PROFILE", "TAGS", "CATEGORY", "API and data contracts", "*"},
		},
		{
			name:      "defaults",
			arguments: []string{"-defaults"},
			contains:  []string{"oncepolicy", "core"},
			excludes:  []string{"determinism", "borrowedstorage"},
		},
		{
			name:      "opt-in",
			arguments: []string{"-opt-in"},
			contains:  []string{"determinism", "extended", "borrowedstorage", "experimental"},
			excludes:  []string{"oncepolicy", "lockorder"},
		},
		{
			name:      "checks",
			arguments: []string{"-checks"},
			contains: []string{
				"CHECK",
				"KIND",
				"TIER",
				"GROUP",
				"policy",
				"hazard",
				"extended",
				"oncepolicy/discarded-wrapper",
				"defect",
				"core",
				"goroutineownership/detached",
				"experimental",
				"reliability",
			},
			excludes: []string{"ANALYZER", "PROFILE", "TAGS", "CATEGORY", "*"},
		},
		{
			name:      "default checks",
			arguments: []string{"-checks", "-defaults"},
			contains:  []string{"lockorder/missing-release"},
			excludes:  []string{"goroutineownership/detached"},
		},
		{
			name:      "opt-in checks",
			arguments: []string{"-checks", "-opt-in"},
			contains:  []string{"goroutineownership/detached"},
			excludes:  []string{"lockorder/missing-release"},
		},
		{name: "conflicting filters", arguments: []string{"-defaults", "-opt-in"}, wantError: true},
		{name: "unexpected argument", arguments: []string{"extra"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkPrintedOutput(t, printAnalyzerList, test.arguments, test.contains, test.excludes, test.wantError)
		})
	}
}

// checkPrintedOutput runs a printing command and checks its error outcome
// and which fragments its output contains.
func checkPrintedOutput(
	t *testing.T,
	print func([]string, io.Writer, io.Writer) error,
	arguments, contains, excludes []string,
	wantError bool,
) {
	t.Helper()
	var output, errorsOutput bytes.Buffer
	err := print(arguments, &output, &errorsOutput)
	if (err != nil) != wantError {
		t.Fatalf("error = %v, wantError %t", err, wantError)
	}
	for _, value := range contains {
		if !strings.Contains(output.String(), value) {
			t.Errorf("output does not contain %q:\n%s", value, output.String())
		}
	}
	for _, value := range excludes {
		if strings.Contains(output.String(), value) {
			t.Errorf("output unexpectedly contains %q:\n%s", value, output.String())
		}
	}
}

func TestRunCLIImmediateCommands(t *testing.T) {
	tests := []struct {
		name           string
		arguments      []string
		wantCode       int
		outputContains string
		errorContains  string
	}{
		{name: "version", arguments: []string{"gohawk", "-V"}, outputContains: "gohawk "},
		{name: "list", arguments: []string{"gohawk", "list", "-defaults"}, outputContains: "oncepolicy"},
		{name: "list error", arguments: []string{"gohawk", "list", "extra"}, wantCode: 2, errorContains: "unexpected argument"},
		{name: "documentation", arguments: []string{"gohawk", "doc", "lockorder/missing-release"}, outputContains: "Reports return paths"},
		{name: "documentation error", arguments: []string{"gohawk", "doc", "unknown"}, wantCode: 2, errorContains: "unknown analyzer or check"},
		{name: "help", arguments: []string{"gohawk", "help"}, errorContains: "Analyzer selection:"},
		{name: "invalid check", arguments: []string{"gohawk", "-disable-checks=unknown/check", "./..."}, wantCode: 2, errorContains: "unknown check"},
		{name: "legacy selection", arguments: []string{"gohawk", "-determinism=false", "./..."}, wantCode: 2, errorContains: "use -disable=determinism"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output, errorsOutput bytes.Buffer
			runtime := testCLIRuntime(t, &output, &errorsOutput)
			result := runCLI(test.arguments, runtime)
			if result.exitCode != test.wantCode {
				t.Fatalf("exit code = %d, want %d", result.exitCode, test.wantCode)
			}
			if result.invocation != nil {
				t.Fatal("immediate command unexpectedly prepared an analysis invocation")
			}
			if test.outputContains != "" && !strings.Contains(output.String(), test.outputContains) {
				t.Errorf("output does not contain %q:\n%s", test.outputContains, output.String())
			}
			if test.errorContains != "" && !strings.Contains(errorsOutput.String(), test.errorContains) {
				t.Errorf("error output does not contain %q:\n%s", test.errorContains, errorsOutput.String())
			}
		})
	}
}

func TestRunCLIProcessBoundaries(t *testing.T) {
	t.Run("filtered flags", func(t *testing.T) {
		var output, errorsOutput bytes.Buffer
		runtime := testCLIRuntime(t, &output, &errorsOutput)
		runtime.filteredFlags = func(arguments []string, analyzers []*analysis.Analyzer, output, errorsOutput io.Writer) int {
			if !slices.Equal(arguments, []string{"gohawk", "-flags"}) {
				t.Errorf("arguments = %v", arguments)
			}
			if len(analyzers) == 0 {
				t.Error("no analyzers supplied")
			}
			_, _ = fmt.Fprint(output, "filtered flags")
			return 7
		}
		result := runCLI([]string{"gohawk", "-flags"}, runtime)
		if result.exitCode != 7 || result.invocation != nil || output.String() != "filtered flags" {
			t.Fatalf("result = %#v, output = %q", result, output.String())
		}
	})

	t.Run("delegated run forwards selection verbatim", func(t *testing.T) {
		var output, errorsOutput bytes.Buffer
		runtime := testCLIRuntime(t, &output, &errorsOutput)
		result := runCLI([]string{"gohawk", "-disable-checks=lockorder/missing-release", "./..."}, runtime)
		invocation := result.invocation
		if invocation == nil || !invocation.delegate || invocation.render != renderRich {
			t.Fatalf("result = %#v", result)
		}
		joined := strings.Join(invocation.arguments, " ")
		for _, want := range []string{"-disable-checks=lockorder/missing-release", "./..."} {
			if !strings.Contains(joined, want) {
				t.Errorf("forwarded arguments do not contain %q: %s", want, joined)
			}
		}
		// The vet-tool children resolve selection; the parent must not expand it
		// into per-analyzer flags go vet would not recognize.
		if strings.Contains(joined, "-lockorder=true") {
			t.Errorf("forwarded arguments must not resolve selection: %s", joined)
		}
	})

	t.Run("vet-tool handshake stays in process", func(t *testing.T) {
		var output, errorsOutput bytes.Buffer
		runtime := testCLIRuntime(t, &output, &errorsOutput)
		result := runCLI([]string{"gohawk", "-disable=oncepolicy", "/tmp/unit.cfg"}, runtime)
		invocation := result.invocation
		if invocation == nil || invocation.delegate {
			t.Fatalf("result = %#v", result)
		}
		joined := strings.Join(invocation.arguments, " ")
		if !strings.Contains(joined, "/tmp/unit.cfg") {
			t.Errorf("handshake arguments lost the unit file: %v", invocation.arguments)
		}
		// Selection is resolved into per-analyzer flags for the unit driver,
		// which is how go vet forwards it to this same handshake: the other
		// analyzers are enabled and the disabled one is left out.
		if !strings.Contains(joined, "-lockorder=true") || strings.Contains(joined, "-oncepolicy=true") {
			t.Errorf("handshake did not disable oncepolicy for the unit driver: %v", invocation.arguments)
		}
	})
}

func TestPrintFilteredFlagsUsing(t *testing.T) {
	analyzers := gohawk.Analyzers()
	execute := func(name string, arguments, environment []string) (processOutput, error) {
		if name != "gohawk" || !slices.Equal(arguments, []string{"-flags"}) {
			t.Errorf("command = %q %v", name, arguments)
		}
		if !slices.Equal(environment, []string{filteredFlagsChild + "=1"}) {
			t.Errorf("environment = %v", environment)
		}
		return processOutput{stdout: []byte(`[
			{"Name":"determinism","Bool":true,"Usage":"legacy selector"},
			{"Name":"enable","Bool":false,"Usage":"enable analyzers"}
		]`)}, nil
	}
	var output, errorsOutput bytes.Buffer
	if code := printFilteredFlagsUsing([]string{"gohawk", "-flags"}, analyzers, &output, &errorsOutput, execute); code != 0 {
		t.Fatalf("exit code = %d\n%s", code, errorsOutput.String())
	}
	if strings.Contains(output.String(), `"Name": "determinism"`) || !strings.Contains(output.String(), `"Name": "enable"`) {
		t.Fatalf("filtered output:\n%s", output.String())
	}

	execute = func(string, []string, []string) (processOutput, error) {
		return processOutput{stderr: []byte("child failed\n"), exitCode: 9}, errors.New("exit status 9")
	}
	output.Reset()
	errorsOutput.Reset()
	if code := printFilteredFlagsUsing([]string{"gohawk", "-flags"}, analyzers, &output, &errorsOutput, execute); code != 9 ||
		errorsOutput.String() != "child failed\n" {
		t.Fatalf("exit code = %d, stderr = %q", code, errorsOutput.String())
	}
}

func TestRenderModeFollowsFlags(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		want      renderMode
		diff      bool
	}{
		{[]string{"gohawk", "./..."}, renderRich, false},
		{[]string{"gohawk", "-json", "./..."}, renderJSON, false},
		{[]string{"gohawk", "-fix", "./..."}, renderFix, false},
		{[]string{"gohawk", "-fix", "-diff", "./..."}, renderFix, true},
	} {
		var output, errorsOutput bytes.Buffer
		runtime := testCLIRuntime(t, &output, &errorsOutput)
		invocation := runCLI(test.arguments, runtime).invocation
		if invocation == nil || !invocation.delegate || invocation.render != test.want || invocation.diff != test.diff {
			t.Errorf("runCLI(%v) invocation = %#v", test.arguments, invocation)
		}
	}
}

func TestRunViaGoVet(t *testing.T) {
	tests := []struct {
		name          string
		render        renderMode
		result        processOutput
		err           error
		wantCode      int
		wantOutput    string
		wantErrOutput string
	}{
		{name: "no diagnostics", render: renderRich, result: processOutput{stdout: []byte(`{}`)}, wantCode: 0},
		{
			name:       "diagnostic",
			render:     renderRich,
			result:     processOutput{stdout: []byte(`{"example.com/p":{"oncepolicy":[{"posn":"missing.go:1:1","message":"problem"}]}}`)},
			wantCode:   3,
			wantOutput: "warning[oncepolicy]: problem",
		},
		{
			// go vet prints one object per package; a pattern matching several
			// packages must render every diagnostic rather than read as a build
			// failure.
			name:   "several packages",
			render: renderRich,
			result: processOutput{stdout: []byte(`{"example.com/p":{"oncepolicy":[{"posn":"p.go:1:1","message":"first"}]}}
{"example.com/q":{"oncepolicy":[{"posn":"q.go:1:1","message":"second"}]}}
`)},
			wantCode:   3,
			wantOutput: "warning[oncepolicy]: second",
		},
		{
			name:       "analysis error",
			render:     renderRich,
			result:     processOutput{stdout: []byte(`{"example.com/p":{"oncepolicy":{"error":"load failed"}}}`)},
			wantCode:   1,
			wantOutput: "oncepolicy: load failed",
		},
		{
			name:       "json passthrough",
			render:     renderJSON,
			result:     processOutput{stdout: []byte(`{"example.com/p":{"oncepolicy":[{"posn":"a.go:1:1","end":"a.go:1:2","message":"m"}]}}`)},
			wantCode:   0,
			wantOutput: `"oncepolicy"`,
		},
		{
			name:          "build failure surfaces stderr",
			render:        renderRich,
			result:        processOutput{stderr: []byte("# p\nbad.go:1: oops\n"), exitCode: 1},
			err:           errors.New("exit status 1"),
			wantCode:      1,
			wantErrOutput: "bad.go:1: oops",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output, errorsOutput bytes.Buffer
			execute := func(name string, arguments, environment []string) (processOutput, error) {
				if name != "go" || !slices.Equal(arguments, []string{"vet", "-vettool=gohawk", "-json", "./..."}) {
					t.Errorf("command = %q %v", name, arguments)
				}
				return test.result, test.err
			}
			runtime := cliRuntime{
				output:       &output,
				errorsOutput: &errorsOutput,
				execute:      execute,
				executable:   func() (string, error) { return "gohawk", nil },
			}
			invocation := &analysisInvocation{delegate: true, arguments: []string{"./..."}, render: test.render}
			if code := runViaGoVet(invocation, runtime); code != test.wantCode {
				t.Fatalf("exit code = %d, want %d\nout=%s\nerr=%s", code, test.wantCode, output.String(), errorsOutput.String())
			}
			if test.wantOutput != "" && !strings.Contains(output.String(), test.wantOutput) {
				t.Errorf("stdout does not contain %q:\n%s", test.wantOutput, output.String())
			}
			if test.wantErrOutput != "" && !strings.Contains(errorsOutput.String(), test.wantErrOutput) {
				t.Errorf("stderr does not contain %q:\n%s", test.wantErrOutput, errorsOutput.String())
			}
		})
	}
}

func testCLIRuntime(t *testing.T, output, errorsOutput io.Writer) cliRuntime {
	t.Helper()
	return cliRuntime{
		output:       output,
		errorsOutput: errorsOutput,
		getenv:       func(string) string { return "" },
		filteredFlags: func([]string, []*analysis.Analyzer, io.Writer, io.Writer) int {
			t.Fatal("unexpected filtered-flags subprocess")
			return 0
		},
		execute: func(string, []string, []string) (processOutput, error) {
			t.Fatal("unexpected analysis subprocess")
			return processOutput{}, nil
		},
		executable: func() (string, error) { return "gohawk", nil },
	}
}

func TestPrintDocumentation(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		contains  []string
		excludes  []string
		wantError bool
	}{
		{
			name:      "analyzer",
			arguments: []string{"lockorder"},
			contains: []string{
				"lockorder", "Group: reliability (reliability and safety)",
				"Suggested fixes: no", "lockorder/missing-release",
				"https://gohawk.dev/analyzers/reliability-and-safety/lockorder/",
			},
			excludes: []string{"lockorder/contradictory-order", "prefer-test-context"},
		},
		{
			name:      "check",
			arguments: []string{"lockorder/missing-release"},
			contains: []string{
				"lockorder/missing-release", "Reports return paths that leave an owned lock held.",
				"Analyzer: lockorder", "Kind: defect", "Group: reliability",
			},
			excludes: []string{"Profile:", "Tags:", "Opt-in:", "\nChecks:", "\nOptions:"},
		},
		{name: "missing target", wantError: true},
		{name: "extra target", arguments: []string{"lockorder", "determinism"}, wantError: true},
		{name: "unknown target", arguments: []string{"unknown"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkPrintedOutput(t, printDocumentation, test.arguments, test.contains, test.excludes, test.wantError)
		})
	}
}
