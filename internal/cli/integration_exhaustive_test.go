package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:cyclop,funlen,gocognit // The opt-in subprocess matrix is a scenario catalog, not one branching algorithm.
func TestCLIIntegrationExhaustive(t *testing.T) {
	if os.Getenv("GOHAWK_EXHAUSTIVE_CLI") == "" {
		t.Skip("set GOHAWK_EXHAUSTIVE_CLI=1 to run the redundant subprocess matrix")
	}
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
			"ownership (ownership and lifecycle): cancellationownership, channelcapacity*, channelownership, channelsafety, deferinloop, " +
				"exitpolicy, goroutineownership, producerlifecycle, processownership, resourcelifetime",
			"reliability (reliability and safety): concurrentcapture, determinism*, errorownership*, errorclassification, " +
				"inlineerror, evalorder, globalstate*, lockorder, oncepolicy, syncmapatomicity, taintpolicy*",
			"testing (testing): testlifecycle*, testpolicy*",
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
		for _, value := range []string{"contextpolicy/nil-context"} {
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

	t.Run("one binary supports Go 1.26 and Go 1.27 modules", func(t *testing.T) {
		for _, fixture := range []struct {
			version string
			source  string
		}{
			{version: "1.26.0", source: `package sample

func identity[T any](value T) T { return value }

func answer() int { return identity(42) }
`},
			// Generic methods require Go 1.27. Keeping one here proves that the
			// analyzer process itself understands the newer language version, not
			// merely that its go command can load a module declaring it.
			{version: "1.27.0", source: `package sample

type identity struct{}

func (identity) value[T any](input T) T { return input }

func answer() int { return identity{}.value(42) }
`},
		} {
			t.Run("go"+fixture.version, func(t *testing.T) {
				compatibilityModule := writeLanguageVersionModule(t, fixture.version, fixture.source)
				output, exitCode := runCommand(t, compatibilityModule, binary, "./...")
				if exitCode != 0 || output != "" {
					t.Fatalf("analyze Go %s module: exit code = %d, output = %q", fixture.version, exitCode, output)
				}
			})
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

		output, exitCode = runCommand(t, modernModule, binary, "-enable-checks=testlifecycle/context-root", "./...")
		if exitCode != 3 || !strings.Contains(output, "test-owned goroutine uses a never-cancelled context") {
			t.Fatalf("Go 1.24 opt-in check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, modernModule, binary, "-enable-all", "./...")
		if exitCode != 3 || !strings.Contains(output, "test-owned goroutine uses a never-cancelled context") {
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
