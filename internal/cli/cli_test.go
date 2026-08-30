package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
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
		{name: "all", contains: []string{"ANALYZER", "GROUP", "apishape*", "oncepolicy", "contracts", "reliability", "* opt-in"}, excludes: []string{"PROFILE", "TAGS", "CATEGORY", "API and data contracts"}},
		{name: "defaults", arguments: []string{"-defaults"}, contains: []string{"oncepolicy"}, excludes: []string{"*", "wirepolicy", "apishape", "blockingtest", "determinism"}},
		{name: "opt-in", arguments: []string{"-opt-in"}, contains: []string{"wirepolicy*", "blockingtest*", "determinism*", "* opt-in"}, excludes: []string{"oncepolicy", "contextpolicy"}},
		{name: "checks", arguments: []string{"-checks"}, contains: []string{"CHECK", "GROUP", "contextpolicy/context-first", "contextpolicy/test-context*", "oncepolicy/discarded-wrapper", "contracts", "* opt-in"}, excludes: []string{"ANALYZER", "PROFILE", "TAGS", "CATEGORY"}},
		{name: "default checks", arguments: []string{"-checks", "-defaults"}, contains: []string{"contextpolicy/context-first"}, excludes: []string{"*", "contextpolicy/test-context", "apishape/parameter-count"}},
		{name: "opt-in checks", arguments: []string{"-checks", "-opt-in"}, contains: []string{"contextpolicy/test-context*", "apishape/parameter-count*", "* opt-in"}, excludes: []string{"contextpolicy/context-first"}},
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
			fmt.Fprint(output, "filtered flags")
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
	if code := printFilteredFlagsUsing([]string{"gohawk", "-flags"}, analyzers, &output, &errorsOutput, execute); code != 9 || errorsOutput.String() != "child failed\n" {
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
		{name: "diagnostic", result: processOutput{stdout: []byte(`{"example.com/p":{"oncepolicy":[{"posn":"missing.go:1:1","end":"missing.go:1:2","message":"problem"}]}}`)}, wantCode: 3, wantOutput: "warning[oncepolicy]: problem"},
		{name: "analysis error", result: processOutput{stdout: []byte(`{"example.com/p":{"oncepolicy":{"error":"load failed"}}}`)}, wantCode: 1, wantOutput: "oncepolicy: load failed"},
		{name: "invalid JSON", result: processOutput{stdout: []byte(`not json`)}, wantCode: 1, wantOutput: "decode analyzer output"},
		{name: "child exit", result: processOutput{stdout: []byte("child output\n"), stderr: []byte("child error\n"), exitCode: 4}, err: errors.New("exit status 4"), wantCode: 4, wantOutput: "child error\nchild output"},
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
				"Suggested fixes: no", "contextpolicy/context-first", "contextpolicy/test-context*", "* opt-in",
				"-contextpolicy.prefer-test-context (default true)",
				"https://gohawk.dev/analyzers/api-and-data-contracts/contextpolicy/",
			},
		},
		{
			name:      "check",
			arguments: []string{"contextpolicy/nil-context"},
			contains: []string{
				"contextpolicy/nil-context", "Reports definitely nil context.Context arguments.",
				"Analyzer: contextpolicy", "Group: contracts",
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

func TestWithAnalyzerSelection(t *testing.T) {
	analyzers := gohawk.Analyzers()
	groups := gohawk.AnalyzerGroups()
	metadata := gohawk.AnalyzerMetadata()
	selectArguments := func(arguments []string) []string {
		t.Helper()
		selected, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false)
		if err != nil {
			t.Fatal(err)
		}
		return selected
	}

	selected := strings.Join(selectArguments([]string{"gohawk", "-enable=wirepolicy", "./..."}), " ")
	if !strings.Contains(selected, "-wirepolicy=true") || strings.Contains(selected, "-enable=") {
		t.Fatalf("selected arguments = %s", selected)
	}
	help := []string{"gohawk", "help", "wirepolicy"}
	if got := selectArguments(help); !slices.Equal(got, help) {
		t.Fatalf("help arguments = %v, want %v", got, help)
	}
	all := []string{"gohawk", "-enable-all", "./..."}
	if got := selectArguments(all); !slices.Equal(got, all) {
		t.Fatalf("enable-all arguments = %v, want %v", got, all)
	}

	got := selectArguments([]string{"gohawk", "-disable=oncepolicy", "./..."})
	joined := strings.Join(got, " ")
	for _, value := range []string{"-contextpolicy=true", "-syncmapatomicity=true"} {
		if !strings.Contains(joined, value) {
			t.Errorf("default arguments do not contain %q: %v", value, got)
		}
	}
	for _, value := range []string{"-oncepolicy=true", "-blockingtest=true", "-determinism=true", "-wirepolicy=true", "-globalstate=true"} {
		if strings.Contains(joined, value) {
			t.Errorf("default arguments unexpectedly contain %q: %v", value, got)
		}
	}

	t.Run("groups include opt-in analyzers", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-groups=testing,contracts", "./..."}), " ")
		for _, value := range []string{"-apishape=true", "-contextpolicy=true", "-closedomain=true", "-wirepolicy=true", "-blockingtest=true", "-testpolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("group arguments do not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-oncepolicy=true", "-channelpolicy=true", "-enable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("group arguments unexpectedly contain %q: %s", value, got)
			}
		}
	})

	t.Run("groups combine with individual selection and exclusion", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-groups", "ownership", "-enable=wirepolicy", "-disable=channelpolicy", "./..."}), " ")
		for _, value := range []string{"-cancellationownership=true", "-goroutineownership=true", "-wirepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("combined arguments do not contain %q: %s", value, got)
			}
		}
		if strings.Contains(got, "-channelpolicy=true") {
			t.Errorf("explicit exclusion did not remove channelpolicy: %s", got)
		}
	})

	t.Run("disabled groups subtract from defaults and allow individual overrides", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-disable-groups=reliability", "-enable=oncepolicy", "./..."}), " ")
		for _, value := range []string{"-contextpolicy=true", "-channelpolicy=true", "-oncepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("disabled-group arguments do not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-concurrentcapture=true", "-errorownership=true", "-disable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("disabled-group arguments unexpectedly contain %q: %s", value, got)
			}
		}
	})

	t.Run("disabled groups subtract from enable-all", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-all", "-disable-groups=testing", "./..."}), " ")
		for _, value := range []string{"-wirepolicy=true", "-oncepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("enable-all exclusion does not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-blockingtest=true", "-testpolicy=true", "-disable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("enable-all exclusion unexpectedly contains %q: %s", value, got)
			}
		}
	})

	t.Run("invalid groups", func(t *testing.T) {
		for _, arguments := range [][]string{
			{"gohawk", "-enable-groups=unknown", "./..."},
			{"gohawk", "-enable-groups=testing,testing", "./..."},
			{"gohawk", "-enable-groups=testing,", "./..."},
			{"gohawk", "-enable-groups="},
			{"gohawk", "-enable-groups"},
			{"gohawk", "-disable-groups=reliability,reliability", "./..."},
			{"gohawk", "-disable-groups=unknown", "./..."},
			{"gohawk", "-enable-groups=testing", "-disable-groups=testing", "./..."},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("arguments %v unexpectedly succeeded", arguments)
			}
		}
	})

	t.Run("invalid analyzer lists", func(t *testing.T) {
		for _, arguments := range [][]string{
			{"gohawk", "-enable=unknown", "./..."},
			{"gohawk", "-enable=wirepolicy,wirepolicy", "./..."},
			{"gohawk", "-disable=oncepolicy,oncepolicy", "./..."},
			{"gohawk", "-enable=wirepolicy", "-disable=wirepolicy", "./..."},
			{"gohawk", "-enable="},
			{"gohawk", "-disable"},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("arguments %v unexpectedly succeeded", arguments)
			}
		}
	})

	t.Run("legacy analyzer Boolean flags", func(t *testing.T) {
		for _, arguments := range [][]string{
			{"gohawk", "-wirepolicy", "./..."},
			{"gohawk", "-oncepolicy=false", "./..."},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("legacy arguments %v unexpectedly succeeded", arguments)
			}
		}
		if _, err := withAnalyzerSelection([]string{"gohawk", "-wirepolicy=true", "./..."}, analyzers, groups, metadata, true); err != nil {
			t.Fatalf("internal analyzer selection failed: %v", err)
		}
	})
}

func TestRequestedDisabledChecks(t *testing.T) {
	metadata := gohawk.AnalyzerMetadata()

	disabled, remaining, err := requestedDisabledChecks([]string{
		"gohawk",
		"-disable-checks=contextpolicy/context-first,contextpolicy/nil-context",
		"./...",
	}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []string{"contextpolicy/context-first", "contextpolicy/nil-context"} {
		if !disabled[check] {
			t.Errorf("disabled checks do not contain %q: %v", check, disabled)
		}
	}
	if want := []string{"gohawk", "./..."}; !slices.Equal(remaining, want) {
		t.Fatalf("remaining arguments = %v, want %v", remaining, want)
	}

	for _, arguments := range [][]string{
		{"gohawk", "-disable-checks=unknown/check", "./..."},
		{"gohawk", "-disable-checks=contextpolicy/context-first,contextpolicy/context-first", "./..."},
		{"gohawk", "-disable-checks=contextpolicy/context-first,", "./..."},
		{"gohawk", "-disable-checks="},
		{"gohawk", "-disable-checks"},
	} {
		if _, _, err := requestedDisabledChecks(arguments, metadata); err == nil {
			t.Errorf("arguments %v unexpectedly succeeded", arguments)
		}
	}
}

func TestRequestedChecks(t *testing.T) {
	metadata := gohawk.AnalyzerMetadata()
	requested, remaining, err := requestedChecks([]string{
		"gohawk",
		"-enable-checks=contextpolicy/test-context,contextpolicy/nil-context",
		"-disable-checks=contextpolicy/context-first",
		"./...",
	}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []string{"contextpolicy/test-context", "contextpolicy/nil-context"} {
		if !requested.enabled[check] {
			t.Errorf("enabled checks do not contain %q: %v", check, requested.enabled)
		}
	}
	if !requested.disabled["contextpolicy/context-first"] {
		t.Errorf("disabled checks = %v", requested.disabled)
	}
	if want := []string{"gohawk", "./..."}; !slices.Equal(remaining, want) {
		t.Fatalf("remaining arguments = %v, want %v", remaining, want)
	}

	for _, arguments := range [][]string{
		{"gohawk", "-enable-checks=unknown/check", "./..."},
		{"gohawk", "-enable-checks=contextpolicy/test-context,contextpolicy/test-context", "./..."},
		{"gohawk", "-enable-checks=contextpolicy/test-context,", "./..."},
		{"gohawk", "-enable-checks="},
		{"gohawk", "-enable-checks"},
	} {
		if _, _, err := requestedChecks(arguments, metadata); err == nil {
			t.Errorf("arguments %v unexpectedly succeeded", arguments)
		}
	}
}

func TestCheckSelectionProfiles(t *testing.T) {
	analyzers := gohawk.Analyzers()
	groups := gohawk.AnalyzerGroups()
	metadata := gohawk.AnalyzerMetadata()
	testContext := "contextpolicy/test-context"
	nilContext := "contextpolicy/nil-context"

	t.Run("check alone selects only that check", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{testContext: true}, disabled: map[string]bool{}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "./..."}, analyzers, groups, metadata, checkOwners(requested.enabled, metadata), false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(selection.arguments, " "), "-contextpolicy=true") || selection.normallySelected["contextpolicy"] {
			t.Fatalf("selection = %+v", selection)
		}
		disabled := effectiveDisabledChecks(metadata, selection.normallySelected, requested, selection.enableAll)
		if disabled[testContext] || !disabled[nilContext] {
			t.Fatalf("disabled checks = %v", disabled)
		}
	})

	t.Run("check adds to selected analyzer defaults", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{testContext: true}, disabled: map[string]bool{}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "-enable=contextpolicy", "./..."}, analyzers, groups, metadata, checkOwners(requested.enabled, metadata), false)
		if err != nil {
			t.Fatal(err)
		}
		disabled := effectiveDisabledChecks(metadata, selection.normallySelected, requested, selection.enableAll)
		if disabled[testContext] || disabled[nilContext] {
			t.Fatalf("disabled checks = %v", disabled)
		}
	})

	t.Run("enable all includes opt-in checks and disable wins", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{}, disabled: map[string]bool{testContext: true}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "-enable-all", "./..."}, analyzers, groups, metadata, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		disabled := effectiveDisabledChecks(metadata, selection.normallySelected, requested, selection.enableAll)
		if !disabled[testContext] || disabled[nilContext] {
			t.Fatalf("disabled checks = %v", disabled)
		}
	})
}

func TestCLIIntegration(t *testing.T) {
	binary := buildTestBinary(t)
	module := writeTestModule(t)

	t.Run("version", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-V")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		if !strings.HasPrefix(output, "gohawk ") || strings.Contains(output, "buildID=") {
			t.Fatalf("human version output = %q", output)
		}

		output, exitCode = runCommand(t, module, binary, "-V=full")
		if exitCode != 0 || !strings.Contains(output, "version devel") || !strings.Contains(output, "buildID=") {
			t.Fatalf("vettool version protocol: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("grouped help", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "help")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, summary := range []string{
			"contracts (API and data contracts): apishape*, contextpolicy, closedomain*, wirepolicy*",
			"ownership (ownership and lifecycle): cancellationownership, channelpolicy, deferinloop, exitpolicy, goroutineownership, processownership, resourcelifetime",
			"reliability (reliability and safety): concurrentcapture, determinism*, errorownership, evalorder, globalstate*, lockorder, oncepolicy, syncmapatomicity, taintpolicy*",
			"testing (testing): blockingtest*, testpolicy*",
		} {
			if !strings.Contains(output, summary) {
				t.Fatalf("help does not contain %q:\n%s", summary, output)
			}
		}
	})

	t.Run("list analyzers", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "list")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"apishape*", "* opt-in", "oncepolicy"} {
			if !strings.Contains(output, value) {
				t.Fatalf("list output does not contain %q:\n%s", value, output)
			}
		}

		output, exitCode = runCommand(t, module, binary, "list", "-defaults")
		if exitCode != 0 || !strings.Contains(output, "oncepolicy") || strings.Contains(output, "wirepolicy") {
			t.Fatalf("default list: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, binary, "list", "-opt-in")
		if exitCode != 0 || !strings.Contains(output, "wirepolicy") || strings.Contains(output, "oncepolicy") {
			t.Fatalf("opt-in list: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("analyzer and check documentation", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "doc", "contextpolicy")
		if exitCode != 0 {
			t.Fatalf("analyzer documentation: exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"contextpolicy/nil-context", "contextpolicy/test-context*", "-contextpolicy.prefer-test-context"} {
			if !strings.Contains(output, value) {
				t.Fatalf("analyzer documentation does not contain %q:\n%s", value, output)
			}
		}

		output, exitCode = runCommand(t, module, binary, "doc", "contextpolicy/nil-context")
		if exitCode != 0 {
			t.Fatalf("check documentation: exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"Reports definitely nil context.Context arguments.", "Analyzer: contextpolicy"} {
			if !strings.Contains(output, value) {
				t.Fatalf("check documentation does not contain %q:\n%s", value, output)
			}
		}
	})

	t.Run("ordinary run", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("default analyzer did not run:\n%s", output)
		}
		for _, value := range []string{"warning[oncepolicy]", "-->", "sample.go:", "^"} {
			if !strings.Contains(output, value) {
				t.Fatalf("rich diagnostic does not contain %q:\n%s", value, output)
			}
		}
		for _, diagnostic := range []string{"persisted or wire struct literal", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("opt-in analyzer unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("selected analyzer", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable=wirepolicy", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "persisted or wire struct literal must use field keys") {
			t.Fatalf("output does not contain wirepolicy diagnostic:\n%s", output)
		}
		if strings.Contains(output, "mutable package state") {
			t.Fatalf("selected analyzer unexpectedly ran globalstate:\n%s", output)
		}
	})

	t.Run("selected analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-groups=contracts", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "persisted or wire struct literal must use field keys") {
			t.Fatalf("contracts group did not run opt-in wirepolicy:\n%s", output)
		}
		for _, diagnostic := range []string{"sync.OnceFunc wrapper is discarded", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("contracts group unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("disabled analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-all", "-disable-groups=reliability", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "persisted or wire struct literal must use field keys") {
			t.Fatalf("enable-all minus reliability did not run wirepolicy:\n%s", output)
		}
		for _, diagnostic := range []string{"sync.OnceFunc wrapper is discarded", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("disabled reliability group unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("all analyzers", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-all", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		for _, diagnostic := range []string{
			"sync.OnceFunc wrapper is discarded",
			"persisted or wire struct literal",
			"mutable package state cache",
		} {
			if !strings.Contains(output, diagnostic) {
				t.Fatalf("all-analyzer output does not contain %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("disabling opt-in analyzer keeps ordinary set", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable=globalstate", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("default analyzers did not run:\n%s", output)
		}
		for _, diagnostic := range []string{"persisted or wire struct literal", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("opt-in analyzer unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("disabled default analyzer", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable=oncepolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("exit code = %d, output = %q", exitCode, output)
		}
	})

	t.Run("target Go version", func(t *testing.T) {
		legacyModule := writeContextTestModule(t, "1.23.0")
		output, exitCode := runCommand(t, legacyModule, binary, "-enable=contextpolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("Go 1.23 module: exit code = %d, output = %q", exitCode, output)
		}

		modernModule := writeContextTestModule(t, "1.24.0")
		output, exitCode = runCommand(t, modernModule, binary, "-enable=contextpolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("Go 1.24 default checks: exit code = %d, output = %q", exitCode, output)
		}

		output, exitCode = runCommand(t, modernModule, binary, "-enable-checks=contextpolicy/test-context", "./...")
		if exitCode != 3 || !strings.Contains(output, "use t.Context() or b.Context()") {
			t.Fatalf("Go 1.24 opt-in check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, modernModule, binary, "-enable-all", "./...")
		if exitCode != 3 || !strings.Contains(output, "use t.Context() or b.Context()") {
			t.Fatalf("Go 1.24 enable-all: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("analyzer configuration", func(t *testing.T) {
		goroutineModule := writeGoroutineTestModule(t)
		output, exitCode := runCommand(t, goroutineModule, binary, "-enable=goroutineownership", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("default policy: exit code = %d, output = %q", exitCode, output)
		}

		output, exitCode = runCommand(t, goroutineModule, binary, "-enable=goroutineownership", "-goroutineownership.mode=join", "./...")
		if exitCode != 3 || !strings.Contains(output, "goroutine is not joined") {
			t.Fatalf("join policy: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, goroutineModule, "go", "vet", "-vettool="+binary, "-enable=goroutineownership", "-goroutineownership.mode=join", "./...")
		if exitCode != 1 || !strings.Contains(output, "goroutine is not joined") {
			t.Fatalf("vettool join policy: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("disabled check", func(t *testing.T) {
		checkModule := writeCheckFilterModule(t)
		output, exitCode := runCommand(t, checkModule, binary, "-enable=contextpolicy", "-disable-checks=contextpolicy/context-first", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if strings.Contains(output, "context.Context must be first parameter") {
			t.Fatalf("disabled check still reported:\n%s", output)
		}
		if !strings.Contains(output, "do not pass nil context.Context") {
			t.Fatalf("enabled sibling check did not report:\n%s", output)
		}

		output, exitCode = runCommand(t, checkModule, "go", "vet", "-vettool="+binary, "-enable=contextpolicy", "-disable-checks=contextpolicy/context-first", "./...")
		if exitCode != 1 || strings.Contains(output, "context.Context must be first parameter") || !strings.Contains(output, "do not pass nil context.Context") {
			t.Fatalf("vettool disabled check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, checkModule, binary,
			"-enable=contextpolicy",
			"-disable-checks=contextpolicy/context-first,contextpolicy/context-storage,contextpolicy/test-context,contextpolicy/nil-context",
			"./...",
		)
		if exitCode != 0 || output != "" {
			t.Fatalf("all checks disabled: exit code = %d, output = %q", exitCode, output)
		}
	})

	t.Run("enabled check", func(t *testing.T) {
		checkModule := writeCheckFilterModule(t)
		output, exitCode := runCommand(t, checkModule, binary, "-enable-checks=contextpolicy/nil-context", "./...")
		if exitCode != 3 || !strings.Contains(output, "do not pass nil context.Context") {
			t.Fatalf("exact check: exit code = %d\n%s", exitCode, output)
		}
		if strings.Contains(output, "context.Context must be first parameter") {
			t.Fatalf("exact check ran default sibling:\n%s", output)
		}

		output, exitCode = runCommand(t, checkModule, "go", "vet", "-vettool="+binary, "-enable-checks=contextpolicy/nil-context", "./...")
		if exitCode != 1 || !strings.Contains(output, "do not pass nil context.Context") || strings.Contains(output, "context.Context must be first parameter") {
			t.Fatalf("vettool exact check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, checkModule, binary,
			"-enable=contextpolicy", "-enable-checks=contextpolicy/test-context", "./...",
		)
		if exitCode != 3 || !strings.Contains(output, "context.Context must be first parameter") || !strings.Contains(output, "do not pass nil context.Context") {
			t.Fatalf("combined analyzer and check selection: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("invalid disabled check", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable-checks=contextpolicy/not-a-check", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown check "contextpolicy/not-a-check"`) {
			t.Fatalf("exit code = %d, want 2 with check error\n%s", exitCode, output)
		}
	})

	t.Run("analyzer flags advertised", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-flags")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, name := range []string{
			"disable",
			"disable-checks",
			"disable-groups",
			"enable",
			"enable-checks",
			"enable-groups",
			"apishape.max-parameters",
			"apishape.max-adjacent-same-type",
			"channelpolicy.max-unexplained-capacity",
			"contextpolicy.prefer-test-context",
			"globalstate.allow-names",
			"globalstate.allow-types",
			"goroutineownership.mode",
			"taintpolicy.sinks",
			"taintpolicy.sanitizers",
			"resourcelifetime.contracts",
			"resourcelifetime.require-reader-close",
		} {
			if !strings.Contains(output, `"Name": "`+name+`"`) {
				t.Fatalf("-flags output does not contain %q:\n%s", name, output)
			}
		}
		for _, name := range []string{"wirepolicy", "oncepolicy", "contextpolicy"} {
			if strings.Contains(output, `"Name": "`+name+`"`) {
				t.Fatalf("-flags output still advertises analyzer Boolean %q:\n%s", name, output)
			}
		}
	})

	t.Run("legacy analyzer Boolean selection", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-wirepolicy=false", "./...")
		if exitCode != 2 || !strings.Contains(output, "use -disable=wirepolicy") {
			t.Fatalf("exit code = %d, want 2 with migration error\n%s", exitCode, output)
		}
	})

	t.Run("invalid analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-groups=unknown", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown analyzer group "unknown"`) {
			t.Fatalf("exit code = %d, want 2 with group error\n%s", exitCode, output)
		}
	})

	t.Run("invalid analyzer option", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable=taintpolicy", "-taintpolicy.sinks=database", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown value "database"`) {
			t.Fatalf("exit code = %d, want 2 with option error\n%s", exitCode, output)
		}
	})

	t.Run("invalid goroutine ownership mode", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable=goroutineownership", "-goroutineownership.mode=strict", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown value "strict"`) {
			t.Fatalf("exit code = %d, want 2 with option error\n%s", exitCode, output)
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-json", "-enable=wirepolicy", "./...")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		var diagnostics map[string]map[string][]json.RawMessage
		if err := json.Unmarshal([]byte(output), &diagnostics); err != nil {
			t.Fatalf("decode JSON output: %v\n%s", err, output)
		}
		if len(diagnostics) == 0 {
			t.Fatal("JSON output contains no package diagnostics")
		}
		count := 0
		for _, analyzers := range diagnostics {
			count += len(analyzers["wirepolicy"])
		}
		if count != 1 {
			t.Fatalf("wirepolicy JSON diagnostic count = %d, want 1\n%s", count, output)
		}
	})

	t.Run("vet tool", func(t *testing.T) {
		output, exitCode := runCommand(t, module, "go", "vet", "-vettool="+binary, "./...")
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1\n%s", exitCode, output)
		}
		if !strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("output does not contain default diagnostic:\n%s", output)
		}
		for _, diagnostic := range []string{"persisted or wire struct literal", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("opt-in analyzer unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-enable=wirepolicy", "./...")
		if exitCode != 1 || !strings.Contains(output, "persisted or wire struct literal") {
			t.Fatalf("vettool opt-in analyzer: exit code = %d\n%s", exitCode, output)
		}
		if strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("vettool selected analyzer unexpectedly ran defaults:\n%s", output)
		}

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-disable=oncepolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("vettool disabled analyzer: exit code = %d, output = %q", exitCode, output)
		}
	})

	t.Run("suggested fix", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable=wirepolicy", "-fix", "-diff", "./...")
		if exitCode != 0 {
			t.Fatalf("preview exit code = %d, want 0\n%s", exitCode, output)
		}
		if !strings.Contains(output, `EventRow{ID: "42", Kind: "created"}`) {
			t.Fatalf("preview does not contain keyed literal:\n%s", output)
		}
		assertFixtureContains(t, module, `EventRow{"42", "created"}`)

		output, exitCode = runCommand(t, module, binary, "-enable=wirepolicy", "-fix", "./...")
		if exitCode != 0 {
			t.Fatalf("fix exit code = %d, want 0\n%s", exitCode, output)
		}
		assertFixtureContains(t, module, `EventRow{ID: "42", Kind: "created"}`)

		output, exitCode = runCommand(t, module, binary, "-enable=wirepolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("fixed module: exit code = %d, output = %q", exitCode, output)
		}
		output, exitCode = runCommand(t, module, "go", "test", "./...")
		if exitCode != 0 {
			t.Fatalf("test fixed module: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("test helper suggested fixes", func(t *testing.T) {
		module := writeTestPolicyFixModule(t)
		const relativePath = "sample/helper_test.go"

		output, exitCode := runCommand(t, module, binary, "-enable=testpolicy", "-fix", "-diff", "./...")
		if exitCode != 0 || strings.Count(output, "t.Helper()") != 2 {
			t.Fatalf("preview exit code = %d, want two helper insertions\n%s", exitCode, output)
		}
		if contents := moduleFileContents(t, module, relativePath); strings.Contains(contents, "t.Helper()") {
			t.Fatalf("preview modified fixture:\n%s", contents)
		}

		output, exitCode = runCommand(t, module, binary, "-enable=testpolicy", "-fix", "./...")
		if exitCode != 0 {
			t.Fatalf("fix exit code = %d, want 0\n%s", exitCode, output)
		}
		contents := moduleFileContents(t, module, relativePath)
		if strings.Count(contents, "t.Helper()") != 2 || !strings.Contains(contents, "// Keep this setup comment.") {
			t.Fatalf("fixed fixture does not contain both helpers and the existing comment:\n%s", contents)
		}

		output, exitCode = runCommand(t, module, binary, "-enable=testpolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("fixed module: exit code = %d, output = %q", exitCode, output)
		}
		output, exitCode = runCommand(t, module, "go", "test", "./...")
		if exitCode != 0 {
			t.Fatalf("test fixed module: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("cancellation ownership through CLI and vet tool", func(t *testing.T) {
		module := writeCancellationFixModule(t)
		const diagnostic = "cancel function from context.WithCancel is not called on every return path"

		output, exitCode := runCommand(t, module, binary, "-enable=cancellationownership", "./...")
		if exitCode != 3 || !strings.Contains(output, diagnostic) {
			t.Fatalf("standalone cancellation diagnostic: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-enable=cancellationownership", "./...")
		if exitCode != 1 || !strings.Contains(output, diagnostic) {
			t.Fatalf("vettool cancellation diagnostic: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, binary, "-fix", "-diff", "./...")
		if exitCode != 0 || !strings.Contains(output, "defer cancel()") {
			t.Fatalf("cancellation fix preview: exit code = %d\n%s", exitCode, output)
		}
		assertFixtureContains(t, module, "_, _ = ctx, cancel")

		output, exitCode = runCommand(t, module, binary, "-fix", "./...")
		if exitCode != 0 {
			t.Fatalf("cancellation fix: exit code = %d\n%s", exitCode, output)
		}
		assertFixtureContains(t, module, "defer cancel()")

		output, exitCode = runCommand(t, module, binary, "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("fixed standalone module: exit code = %d, output = %q", exitCode, output)
		}
		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("fixed vettool module: exit code = %d, output = %q", exitCode, output)
		}
		output, exitCode = runCommand(t, module, "go", "test", "./...")
		if exitCode != 0 {
			t.Fatalf("test fixed module: exit code = %d\n%s", exitCode, output)
		}
	})
}

func TestHumanVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1"}}
	if got := humanVersion(info, true); got != "v0.1.1" {
		t.Fatalf("humanVersion() = %q, want v0.1.1", got)
	}
	for _, test := range []struct {
		info *debug.BuildInfo
		ok   bool
	}{
		{info: nil, ok: false},
		{info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ok: true},
		{info: &debug.BuildInfo{}, ok: true},
	} {
		if got := humanVersion(test.info, test.ok); got != "devel" {
			t.Errorf("humanVersion(%#v, %t) = %q, want devel", test.info, test.ok, got)
		}
	}
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	name := "gohawk"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	output, exitCode := runCommand(t, repositoryRoot, "go", "build", "-o", binary, ".")
	if exitCode != 0 {
		t.Fatalf("build CLI: exit code = %d\n%s", exitCode, output)
	}
	return binary
}

func writeTestModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/gohawkcli\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), `package sample

import "sync"

type EventRow struct {
	ID   string `+"`json:\"id\"`"+`
	Kind string `+"`json:\"kind\"`"+`
}

var event = EventRow{"42", "created"}

var cache = map[string]string{}

func initialize() {
	sync.OnceFunc(func() {})()
}
`)
	return directory
}

func writeCancellationFixModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/cancellationfix\n\ngo 1.26.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), `package cancellationfix

import "context"

func work(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	_, _ = ctx, cancel
}
`)
	writeTestFile(t, filepath.Join(directory, "sample", "sample_test.go"), `package cancellationfix

import "testing"

func TestWork(t *testing.T) {
	_ = t
}
`)
	return directory
}

func writeTestPolicyFixModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/testpolicyfix\n\ngo 1.26.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "helper_test.go"), `package sample

import "testing"

func requireUser(t *testing.T) {
	// Keep this setup comment.
	t.Log("require user")
}

func emptyHelper(t *testing.T) {}

func TestHelpers(t *testing.T) {
	requireUser(t)
	emptyHelper(t)
}
`)
	return directory
}

func writeContextTestModule(t *testing.T, goVersion string) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/contexttest\n\ngo "+goVersion+"\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), "package sample\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample_test.go"), `package sample

import (
	"context"
	"testing"
)

func TestBackground(t *testing.T) {
	_ = t
	_ = context.Background()
}
`)
	return directory
}

func writeGoroutineTestModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/goroutinetest\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "worker", "worker.go"), `package worker

import "context"

func Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
	}()
}
`)
	return directory
}

func writeCheckFilterModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/checkfilter\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), `package sample

import "context"

func misplaced(value string, ctx context.Context) {}

func accept(ctx context.Context) {}

func call() {
	accept(nil)
}
`)
	return directory
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func assertFixtureContains(t *testing.T, module, want string) {
	t.Helper()
	contents := moduleFileContents(t, module, filepath.Join("sample", "sample.go"))
	if !strings.Contains(contents, want) {
		t.Fatalf("fixture does not contain %q:\n%s", want, contents)
	}
}

func moduleFileContents(t *testing.T, module, relativePath string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(module, relativePath))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(contents)
}

func runCommand(t *testing.T, directory, name string, arguments ...string) (string, int) {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return string(output), exitError.ExitCode()
	}
	t.Fatalf("run %s: %v", name, err)
	return "", -1
}
