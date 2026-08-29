package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		{name: "all", contains: []string{"ANALYZER", "PROFILE", "TAGS", "apishape", "policy", "oncepolicy", "correctness"}},
		{name: "defaults", arguments: []string{"-defaults"}, contains: []string{"oncepolicy", "default", "correctness"}, excludes: []string{"wirepolicy", "apishape"}},
		{name: "opt-in", arguments: []string{"-opt-in"}, contains: []string{"wirepolicy", "opt-in", "reliability,policy"}, excludes: []string{"oncepolicy", "contextpolicy"}},
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

func TestWithDefaultAnalyzerSelection(t *testing.T) {
	analyzers := gohawk.Analyzers()
	metadata := gohawk.AnalyzerMetadata()

	selected := []string{"gohawk", "-wirepolicy", "./..."}
	if got := withDefaultAnalyzerSelection(selected, analyzers, metadata); !slices.Equal(got, selected) {
		t.Fatalf("selected arguments = %v, want %v", got, selected)
	}
	help := []string{"gohawk", "help", "wirepolicy"}
	if got := withDefaultAnalyzerSelection(help, analyzers, metadata); !slices.Equal(got, help) {
		t.Fatalf("help arguments = %v, want %v", got, help)
	}
	all := []string{"gohawk", "-enable-all", "./..."}
	if got := withDefaultAnalyzerSelection(all, analyzers, metadata); !slices.Equal(got, all) {
		t.Fatalf("enable-all arguments = %v, want %v", got, all)
	}

	got := withDefaultAnalyzerSelection([]string{"gohawk", "-determinism=false", "./..."}, analyzers, metadata)
	joined := strings.Join(got, " ")
	for _, value := range []string{"-contextpolicy=true", "-oncepolicy=true"} {
		if !strings.Contains(joined, value) {
			t.Errorf("default arguments do not contain %q: %v", value, got)
		}
	}
	for _, value := range []string{"-determinism=true", "-wirepolicy=true", "-globalstate=true"} {
		if strings.Contains(joined, value) {
			t.Errorf("default arguments unexpectedly contain %q: %v", value, got)
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
		if !strings.Contains(output, "version") {
			t.Fatalf("output does not contain version information:\n%s", output)
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
			"reliability (reliability and safety): concurrentcapture, determinism, errorownership, evalorder, globalstate (opt-in), lockorder, oncepolicy, syncmapatomicity, taintpolicy (opt-in)",
			"testing (testing): blockingtest, testpolicy (opt-in)",
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
		output, exitCode := runCommand(t, module, binary, "-wirepolicy", "./...")
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
		output, exitCode := runCommand(t, module, binary, "-globalstate=false", "./...")
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
		output, exitCode := runCommand(t, module, binary, "-oncepolicy=false", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("exit code = %d, output = %q", exitCode, output)
		}
	})

	t.Run("target Go version", func(t *testing.T) {
		legacyModule := writeContextTestModule(t, "1.23.0")
		output, exitCode := runCommand(t, legacyModule, binary, "-contextpolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("Go 1.23 module: exit code = %d, output = %q", exitCode, output)
		}

		modernModule := writeContextTestModule(t, "1.24.0")
		output, exitCode = runCommand(t, modernModule, binary, "-contextpolicy", "./...")
		if exitCode != 3 || !strings.Contains(output, "use t.Context() or b.Context()") {
			t.Fatalf("Go 1.24 module: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("analyzer configuration", func(t *testing.T) {
		goroutineModule := writeGoroutineTestModule(t)
		output, exitCode := runCommand(t, goroutineModule, binary, "-goroutineownership", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("default policy: exit code = %d, output = %q", exitCode, output)
		}

		output, exitCode = runCommand(t, goroutineModule, binary, "-goroutineownership", "-goroutineownership.mode=join", "./...")
		if exitCode != 3 || !strings.Contains(output, "goroutine is not joined") {
			t.Fatalf("join policy: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, goroutineModule, "go", "vet", "-vettool="+binary, "-goroutineownership", "-goroutineownership.mode=join", "./...")
		if exitCode != 1 || !strings.Contains(output, "goroutine is not joined") {
			t.Fatalf("vettool join policy: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("analyzer flags advertised", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-flags")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, name := range []string{
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
	})

	t.Run("invalid analyzer option", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-taintpolicy", "-taintpolicy.sinks=database", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown value "database"`) {
			t.Fatalf("exit code = %d, want 2 with option error\n%s", exitCode, output)
		}
	})

	t.Run("invalid goroutine ownership mode", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-goroutineownership", "-goroutineownership.mode=strict", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown value "strict"`) {
			t.Fatalf("exit code = %d, want 2 with option error\n%s", exitCode, output)
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-json", "-wirepolicy", "./...")
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

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-wirepolicy", "./...")
		if exitCode != 1 || !strings.Contains(output, "persisted or wire struct literal") {
			t.Fatalf("vettool opt-in analyzer: exit code = %d\n%s", exitCode, output)
		}
		if strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("vettool selected analyzer unexpectedly ran defaults:\n%s", output)
		}
	})

	t.Run("suggested fix", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-wirepolicy", "-fix", "-diff", "./...")
		if exitCode != 0 {
			t.Fatalf("preview exit code = %d, want 0\n%s", exitCode, output)
		}
		if !strings.Contains(output, `EventRow{ID: "42", Kind: "created"}`) {
			t.Fatalf("preview does not contain keyed literal:\n%s", output)
		}
		assertFixtureContains(t, module, `EventRow{"42", "created"}`)

		output, exitCode = runCommand(t, module, binary, "-wirepolicy", "-fix", "./...")
		if exitCode != 0 {
			t.Fatalf("fix exit code = %d, want 0\n%s", exitCode, output)
		}
		assertFixtureContains(t, module, `EventRow{ID: "42", Kind: "created"}`)

		output, exitCode = runCommand(t, module, binary, "-wirepolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("fixed module: exit code = %d, output = %q", exitCode, output)
		}
	})
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
