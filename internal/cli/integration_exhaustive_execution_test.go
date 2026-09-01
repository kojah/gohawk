package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:cyclop,funlen,gocognit // Independent subprocess scenarios form one opt-in execution catalog.
func runExhaustiveExecutionScenarios(t *testing.T, binary, module string) {
	t.Helper()

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

		output, exitCode = runCommand(
			t,
			goroutineModule,
			"go",
			"vet",
			"-vettool="+binary,
			"-enable=goroutineownership",
			"-goroutineownership.mode=join",
			"./...",
		)
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

		output, exitCode = runCommand(
			t,
			checkModule,
			"go",
			"vet",
			"-vettool="+binary,
			"-enable=contextpolicy",
			"-disable-checks=contextpolicy/context-first",
			"./...",
		)
		if exitCode != 1 || strings.Contains(output, "context.Context must be first parameter") ||
			!strings.Contains(output, "do not pass nil context.Context") {
			t.Fatalf("vettool disabled check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, checkModule, binary,
			"-enable=contextpolicy",
			"-disable-checks=contextpolicy/context-first,contextpolicy/context-storage,contextpolicy/nil-context",
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
		if exitCode != 1 || !strings.Contains(output, "do not pass nil context.Context") ||
			strings.Contains(output, "context.Context must be first parameter") {
			t.Fatalf("vettool exact check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, checkModule, binary,
			"-enable=contextpolicy", "-enable-checks=testlifecycle/context-root", "./...",
		)
		if exitCode != 3 || !strings.Contains(output, "context.Context must be first parameter") ||
			!strings.Contains(output, "do not pass nil context.Context") {
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
			"channelcapacity.max-unexplained-capacity",
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
		tracePath := filepath.Join(t.TempDir(), "evidence.jsonl")

		output, exitCode := runCommand(
			t,
			module,
			binary,
			"-json",
			"-enable=cancellationownership",
			"-gohawk-trace=cancellationownership",
			"-gohawk-trace-file="+tracePath,
			"./...",
		)
		if exitCode != 0 || !json.Valid([]byte(output)) {
			t.Fatalf("traced JSON run: exit code = %d\n%s", exitCode, output)
		}
		traceOutput, err := os.ReadFile(tracePath)
		if err != nil {
			t.Fatalf("read evidence trace: %v", err)
		}
		found := map[string]bool{}
		for _, line := range bytes.Split(bytes.TrimSpace(traceOutput), []byte("\n")) {
			var event struct {
				Analyzer string `json:"analyzer"`
				Phase    string `json:"phase"`
				Reason   string `json:"reason"`
				Outcome  string `json:"outcome"`
			}
			if err := json.Unmarshal(line, &event); err != nil {
				t.Fatalf("decode evidence trace line %q: %v", line, err)
			}
			if event.Analyzer == "cancellationownership" {
				found[event.Phase+"/"+event.Reason+"/"+event.Outcome] = true
			}
		}
		for _, want := range []string{
			"candidate/diagnostic-candidate/observed",
			"decision/unowned-return/rejected",
			"decision/diagnostic-reported/rejected",
			"fix/suggested-fix-available/accepted",
		} {
			if !found[want] {
				t.Fatalf("trace does not contain %s:\n%s", want, traceOutput)
			}
		}

		output, exitCode = runCommand(t, module, binary, "-enable=cancellationownership", "./...")
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
