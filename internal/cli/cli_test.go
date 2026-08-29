package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	gohawk "github.com/kojah/gohawk/analyzers"
)

func TestPrintAnalyzerList(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		contains  []string
		excludes  []string
		wantError bool
	}{
		{name: "all", contains: []string{"ANALYZER", "PROFILE", "GROUP", "apishape", "oncepolicy", "contracts", "reliability"}, excludes: []string{"CATEGORY", "CHECK TAGS", "API and data contracts", "correctness,reliability"}},
		{name: "defaults", arguments: []string{"-defaults"}, contains: []string{"oncepolicy", "default"}, excludes: []string{"wirepolicy", "apishape", "blockingtest", "determinism"}},
		{name: "opt-in", arguments: []string{"-opt-in"}, contains: []string{"wirepolicy", "blockingtest", "determinism", "opt-in"}, excludes: []string{"oncepolicy", "contextpolicy"}},
		{name: "checks", arguments: []string{"-checks"}, contains: []string{"CHECK", "GROUP", "TAGS", "contextpolicy/context-first", "oncepolicy/discarded-wrapper", "contracts", "correctness", "reliability,policy"}, excludes: []string{"ANALYZER", "CATEGORY"}},
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
				"contextpolicy", "Profile: default", "Group: contracts (API and data contracts)",
				"Suggested fixes: no", "contextpolicy/context-first", "Tags: reliability,policy",
				"-contextpolicy.prefer-test-context (default true)",
				"https://kojah.github.io/gohawk/analyzers/api-and-data-contracts/contextpolicy/",
			},
		},
		{
			name:      "check",
			arguments: []string{"contextpolicy/nil-context"},
			contains: []string{
				"contextpolicy/nil-context", "Reports definitely nil context.Context arguments.",
				"Analyzer: contextpolicy", "Profile: default", "Group: contracts",
				"correctness — Strong evidence that the program can behave incorrectly.",
			},
			excludes: []string{"\nChecks:", "\nOptions:"},
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
			"contracts (API and data contracts): apishape (opt-in), contextpolicy, closedomain (opt-in), wirepolicy (opt-in)",
			"ownership (ownership and lifecycle): cancellationownership, channelpolicy, deferinloop, exitpolicy, goroutineownership, processownership, resourcelifetime",
			"reliability (reliability and safety): concurrentcapture, determinism (opt-in), errorownership, evalorder, globalstate (opt-in), lockorder, oncepolicy, syncmapatomicity, taintpolicy (opt-in)",
			"testing (testing): blockingtest (opt-in), testpolicy (opt-in)",
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
		for _, value := range []string{"apishape", "opt-in", "oncepolicy", "default"} {
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
		for _, value := range []string{"Profile: default", "contextpolicy/nil-context", "-contextpolicy.prefer-test-context"} {
			if !strings.Contains(output, value) {
				t.Fatalf("analyzer documentation does not contain %q:\n%s", value, output)
			}
		}

		output, exitCode = runCommand(t, module, binary, "doc", "contextpolicy/nil-context")
		if exitCode != 0 {
			t.Fatalf("check documentation: exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"Reports definitely nil context.Context arguments.", "correctness — Strong evidence"} {
			if !strings.Contains(output, value) {
				t.Fatalf("check documentation does not contain %q:\n%s", value, output)
			}
		}
	})

	t.Run("default profile", func(t *testing.T) {
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

	t.Run("disabling opt-in analyzer keeps default profile", func(t *testing.T) {
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
		if exitCode != 3 || !strings.Contains(output, "use t.Context() or b.Context()") {
			t.Fatalf("Go 1.24 module: exit code = %d\n%s", exitCode, output)
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
	contents, err := os.ReadFile(filepath.Join(module, "sample", "sample.go"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !strings.Contains(string(contents), want) {
		t.Fatalf("fixture does not contain %q:\n%s", want, contents)
	}
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
