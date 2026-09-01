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
			contains: []string{"ANALYZER", "GROUP", "apishape*", "oncepolicy", "contracts", "reliability", "* opt-in"},
			excludes: []string{"PROFILE", "TAGS", "CATEGORY", "API and data contracts"},
		},
		{
			name:      "defaults",
			arguments: []string{"-defaults"},
			contains:  []string{"oncepolicy"},
			excludes:  []string{"*", "wirepolicy", "apishape", "determinism"},
		},
		{
			name:      "opt-in",
			arguments: []string{"-opt-in"},
			contains:  []string{"wirepolicy*", "determinism*", "* opt-in"},
			excludes:  []string{"oncepolicy", "contextpolicy"},
		},
		{
			name:      "checks",
			arguments: []string{"-checks"},
			contains: []string{
				"CHECK",
				"KIND",
				"GROUP",
				"contextpolicy/context-first",
				"policy",
				"testlifecycle/context-root*",
				"hazard",
				"oncepolicy/discarded-wrapper",
				"defect",
				"contracts",
				"* opt-in",
			},
			excludes: []string{"ANALYZER", "PROFILE", "TAGS", "CATEGORY"},
		},
		{
			name:      "default checks",
			arguments: []string{"-checks", "-defaults"},
			contains:  []string{"contextpolicy/context-first"},
			excludes:  []string{"*", "testlifecycle/context-root", "apishape/parameter-count"},
		},
		{
			name:      "opt-in checks",
			arguments: []string{"-checks", "-opt-in"},
			contains:  []string{"testlifecycle/context-root*", "apishape/parameter-count*", "* opt-in"},
			excludes:  []string{"contextpolicy/context-first"},
		},
		{name: "conflicting filters", arguments: []string{"-defaults", "-opt-in"}, wantError: true},
		{name: "unexpected argument", arguments: []string{"extra"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output, errorsOutput bytes.Buffer
			err := printAnalyzerList(test.arguments, &output, &errorsOutput)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %t", err, test.wantError)
			}
			for _, value := range test.contains {
				if !strings.Contains(output.String(), value) {
					t.Errorf("output does not contain %q:\n%s", value, output.String())
				}
			}
			for _, value := range test.excludes {
				if strings.Contains(output.String(), value) {
					t.Errorf("output unexpectedly contains %q:\n%s", value, output.String())
				}
			}
		})
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
		{name: "documentation", arguments: []string{"gohawk", "doc", "contextpolicy/nil-context"}, outputContains: "Reports definitely nil"},
		{name: "documentation error", arguments: []string{"gohawk", "doc", "unknown"}, wantCode: 2, errorContains: "unknown analyzer or check"},
		{name: "help", arguments: []string{"gohawk", "help"}, errorContains: "Analyzer selection:"},
		{name: "invalid check", arguments: []string{"gohawk", "-disable-checks=unknown/check", "./..."}, wantCode: 2, errorContains: "unknown check"},
		{name: "legacy selection", arguments: []string{"gohawk", "-wirepolicy=false", "./..."}, wantCode: 2, errorContains: "use -disable=wirepolicy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output, errorsOutput bytes.Buffer
			runtime := testCLIRuntime(t, &output, &errorsOutput, nil)
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
		runtime := testCLIRuntime(t, &output, &errorsOutput, nil)
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

	t.Run("rich output", func(t *testing.T) {
		var output, errorsOutput bytes.Buffer
		runtime := testCLIRuntime(t, &output, &errorsOutput, nil)
		var childArguments []string
		runtime.richOutput = func(arguments []string, output io.Writer) int {
			childArguments = slices.Clone(arguments)
			return 3
		}
		result := runCLI([]string{"gohawk", "-disable-checks=contextpolicy/context-first", "./..."}, runtime)
		if result.exitCode != 3 || result.invocation != nil {
			t.Fatalf("result = %#v", result)
		}
		joined := strings.Join(childArguments, " ")
		for _, want := range []string{"-contextpolicy=true", "-disable-checks=", "contextpolicy/context-first", "./..."} {
			if !strings.Contains(joined, want) {
				t.Errorf("child arguments do not contain %q: %s", want, joined)
			}
		}
	})

	t.Run("analysis engine", func(t *testing.T) {
		var output, errorsOutput bytes.Buffer
		runtime := testCLIRuntime(t, &output, &errorsOutput, map[string]string{richOutputChild: "1"})
		result := runCLI([]string{"gohawk", "-disable=oncepolicy", "./..."}, runtime)
		if result.exitCode != 0 || result.invocation == nil {
			t.Fatalf("result = %#v", result)
		}
		joined := strings.Join(result.invocation.arguments, " ")
		if !strings.Contains(joined, "-contextpolicy=true") || strings.Contains(joined, "-oncepolicy=true") {
			t.Fatalf("engine arguments = %s", joined)
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
			{"Name":"wirepolicy","Bool":true,"Usage":"legacy selector"},
			{"Name":"enable","Bool":false,"Usage":"enable analyzers"}
		]`)}, nil
	}
	var output, errorsOutput bytes.Buffer
	if code := printFilteredFlagsUsing([]string{"gohawk", "-flags"}, analyzers, &output, &errorsOutput, execute); code != 0 {
		t.Fatalf("exit code = %d\n%s", code, errorsOutput.String())
	}
	if strings.Contains(output.String(), `"Name": "wirepolicy"`) || !strings.Contains(output.String(), `"Name": "enable"`) {
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

func TestRunWithRichOutputUsing(t *testing.T) {
	tests := []struct {
		name       string
		result     processOutput
		err        error
		wantCode   int
		wantOutput string
	}{
		{name: "no diagnostics", result: processOutput{stdout: []byte(`{}`)}, wantCode: 0},
		{
			name: "diagnostic",
			result: processOutput{
				stdout: []byte(`{"example.com/p":{"oncepolicy":[{"posn":"missing.go:1:1","end":"missing.go:1:2","message":"problem"}]}}`),
			},
			wantCode:   3,
			wantOutput: "warning[oncepolicy]: problem",
		},
		{
			name:       "analysis error",
			result:     processOutput{stdout: []byte(`{"example.com/p":{"oncepolicy":{"error":"load failed"}}}`)},
			wantCode:   1,
			wantOutput: "oncepolicy: load failed",
		},
		{name: "invalid JSON", result: processOutput{stdout: []byte(`not json`)}, wantCode: 1, wantOutput: "decode analyzer output"},
		{
			name:       "child exit",
			result:     processOutput{stdout: []byte("child output\n"), stderr: []byte("child error\n"), exitCode: 4},
			err:        errors.New("exit status 4"),
			wantCode:   4,
			wantOutput: "child error\nchild output",
		},
		{name: "start error", result: processOutput{exitCode: -1}, err: errors.New("not found"), wantCode: 1, wantOutput: "run analyzer engine: not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execute := func(name string, arguments, environment []string) (processOutput, error) {
				if name != "gohawk" || !slices.Equal(arguments, []string{"-json", "./..."}) {
					t.Errorf("command = %q %v", name, arguments)
				}
				if !slices.Equal(environment, []string{richOutputChild + "=1"}) {
					t.Errorf("environment = %v", environment)
				}
				return test.result, test.err
			}
			var output bytes.Buffer
			if code := runWithRichOutputUsing([]string{"gohawk", "./..."}, &output, execute); code != test.wantCode {
				t.Fatalf("exit code = %d, want %d\n%s", code, test.wantCode, output.String())
			}
			if test.wantOutput != "" && !strings.Contains(output.String(), test.wantOutput) {
				t.Errorf("output does not contain %q:\n%s", test.wantOutput, output.String())
			}
		})
	}
}

func testCLIRuntime(t *testing.T, output, errorsOutput io.Writer, environment map[string]string) cliRuntime {
	t.Helper()
	return cliRuntime{
		output:       output,
		errorsOutput: errorsOutput,
		getenv: func(name string) string {
			return environment[name]
		},
		filteredFlags: func([]string, []*analysis.Analyzer, io.Writer, io.Writer) int {
			t.Fatal("unexpected filtered-flags subprocess")
			return 0
		},
		richOutput: func([]string, io.Writer) int {
			t.Fatal("unexpected rich-output subprocess")
			return 0
		},
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
			arguments: []string{"contextpolicy"},
			contains: []string{
				"contextpolicy", "Group: contracts (API and data contracts)",
				"Suggested fixes: no", "contextpolicy/context-first",
				"https://gohawk.dev/analyzers/api-and-data-contracts/contextpolicy/",
			},
			excludes: []string{"testlifecycle/context-root", "prefer-test-context"},
		},
		{
			name:      "check",
			arguments: []string{"contextpolicy/nil-context"},
			contains: []string{
				"contextpolicy/nil-context", "Reports definitely nil context.Context arguments.",
				"Analyzer: contextpolicy", "Kind: defect", "Group: contracts",
			},
			excludes: []string{"Profile:", "Tags:", "Opt-in:", "\nChecks:", "\nOptions:"},
		},
		{name: "missing target", wantError: true},
		{name: "extra target", arguments: []string{"contextpolicy", "wirepolicy"}, wantError: true},
		{name: "unknown target", arguments: []string{"unknown"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output, errorsOutput bytes.Buffer
			err := printDocumentation(test.arguments, &output, &errorsOutput)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %t", err, test.wantError)
			}
			for _, value := range test.contains {
				if !strings.Contains(output.String(), value) {
					t.Errorf("output does not contain %q:\n%s", value, output.String())
				}
			}
			for _, value := range test.excludes {
				if strings.Contains(output.String(), value) {
					t.Errorf("output unexpectedly contains %q:\n%s", value, output.String())
				}
			}
		})
	}
}
