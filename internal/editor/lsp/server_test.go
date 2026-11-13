package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// These scenarios drive a real server over pipes against a small module, with
// the gohawk binary as go vet's tool, so they cover what an editor sees:
// which findings are published, for which file, and what the status says.

const scenarioTimeout = 90 * time.Second

const leakSource = `package leak

import (
	"io"
	"os"
)

func ReadHeader(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, err
	}
	f.Close()
	return header, nil
}
`

var fixedSource = strings.Replace(
	strings.Replace(leakSource, "\tf.Close()\n", "", 1),
	"\theader := make", "\tdefer f.Close()\n\theader := make", 1)

var (
	binaryOnce      sync.Once
	binaryDirectory string
	binaryPath      string
	errBinary       error
)

// runTests owns the directory the shared gohawk binary is built into, so it
// outlives any single test and is removed when the package finishes.
func runTests(m *testing.M) {
	directory, err := os.MkdirTemp("", "gohawk-lsp-test-")
	if err == nil {
		binaryDirectory = directory
		defer func() { _ = os.RemoveAll(directory) }()
	}
	m.Run()
}

// testBinary builds gohawk once, the first time a scenario needs it.
func testBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		if binaryDirectory == "" {
			errBinary = errors.New("no directory for the test binary")
			return
		}
		name := "gohawk"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binaryPath = filepath.Join(binaryDirectory, name)
		build := exec.CommandContext(context.Background(), "go", "build", "-o", binaryPath, "github.com/kojah/gohawk")
		if output, err := build.CombinedOutput(); err != nil {
			errBinary = fmt.Errorf("%w\n%s", err, output)
		}
	})
	if errBinary != nil {
		t.Fatalf("build gohawk: %v", errBinary)
	}
	return binaryPath
}

type editor struct {
	t           *testing.T
	connection  *connection
	diagnostics chan publishDiagnosticsParams
	statuses    chan StatusParams
	replies     chan *message
	nextID      int
	logMutex    sync.Mutex
	logLines    []string
}

// logs returns what the server has written to its output log, for a failing
// scenario to show.
func (e *editor) logs() string {
	e.logMutex.Lock()
	defer e.logMutex.Unlock()
	return strings.Join(e.logLines, "\n")
}

// lockedBuffer lets the test read the event log while the server writes it.
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) events(t *testing.T) []event {
	t.Helper()
	b.mutex.Lock()
	defer b.mutex.Unlock()
	var events []event
	for line := range strings.Lines(b.buffer.String()) {
		var next event
		if err := json.Unmarshal([]byte(line), &next); err != nil {
			t.Fatalf("event log line %q: %v", line, err)
		}
		events = append(events, next)
	}
	return events
}

type editorOptions struct {
	eventLog       io.Writer
	settings       map[string]any
	clientCommands []string
	workspace      string
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files["go.mod"] = "module example.com/editor\n\ngo 1.25.0\n"
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func startEditor(t *testing.T, options editorOptions) *editor {
	t.Helper()
	binary := testBinary(t)
	toServer, fromEditor := io.Pipe()
	toEditor, fromServer := io.Pipe()
	served := make(chan error, 1)
	go func() {
		served <- Serve(t.Context(), toServer, fromServer, Options{
			Executable: binary,
			EventLog:   options.eventLog,
			Catalog: func(check string) CheckInfo {
				return CheckInfo{
					Help: "help for " + check, DocumentationURL: "https://example.com/" + check,
					Experimental: strings.HasPrefix(check, "processownership/"),
				}
			},
		})
		// Closing the pipe ends the editor's read loop; it cannot fail.
		_ = fromServer.Close()
	}()
	client := &editor{
		t:           t,
		connection:  newConnection(toEditor, fromEditor),
		diagnostics: make(chan publishDiagnosticsParams, 64),
		statuses:    make(chan StatusParams, 256),
		replies:     make(chan *message, 16),
	}
	go client.receive()
	t.Cleanup(func() {
		if err := client.connection.notify(methodExit, nil); err != nil {
			t.Errorf("send exit: %v", err)
		}
		select {
		case err := <-served:
			if err != nil {
				t.Errorf("server stopped with %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("server did not stop after exit")
		}
		_ = fromEditor.Close()
	})
	settings := map[string]any{"debounce": 50}
	maps.Copy(settings, options.settings)
	client.call(methodInitialize, map[string]any{
		"rootUri":               uriFromPath(options.workspace),
		"initializationOptions": map[string]any{"settings": settings, "clientCommands": options.clientCommands},
	})
	return client
}

func (e *editor) receive() {
	for {
		incoming, err := e.connection.read()
		if err != nil {
			return
		}
		switch incoming.Method {
		case methodPublishDiagnostics:
			var params publishDiagnosticsParams
			if decode(incoming.Params, &params) {
				e.diagnostics <- params
			}
		case methodStatus:
			var params StatusParams
			if decode(incoming.Params, &params) {
				e.statuses <- params
			}
		case methodLogMessage:
			var params logMessageParams
			if decode(incoming.Params, &params) {
				e.logMutex.Lock()
				e.logLines = append(e.logLines, params.Message)
				e.logMutex.Unlock()
			}
		case "":
			e.replies <- incoming
		}
	}
}

func (e *editor) call(method string, params any) json.RawMessage {
	e.t.Helper()
	e.nextID++
	encoded, err := json.Marshal(params)
	if err != nil {
		e.t.Fatal(err)
	}
	id := json.RawMessage(strings.TrimSpace(string(mustJSON(e.t, e.nextID))))
	if err := e.connection.write(&message{ID: &id, Method: method, Params: encoded}); err != nil {
		e.t.Fatal(err)
	}
	select {
	case reply := <-e.replies:
		if reply.Error != nil {
			e.t.Fatalf("%s: %s", method, reply.Error.Message)
		}
		return reply.Result
	case <-time.After(scenarioTimeout):
		e.t.Fatalf("no reply to %s", method)
		return nil
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func (e *editor) notify(method string, params any) {
	e.t.Helper()
	if err := e.connection.notify(method, params); err != nil {
		e.t.Fatal(err)
	}
}

func (e *editor) open(path, text string) {
	e.notify(methodDidOpen, didOpenParams{TextDocument: textDocumentItem{URI: uriFromPath(path), Version: 1, Text: text}})
}

func (e *editor) change(path, text string, version int) {
	e.notify(methodDidChange, map[string]any{
		"textDocument":   map[string]any{"uri": uriFromPath(path), "version": version},
		"contentChanges": []contentChange{{Text: text}},
	})
}

func (e *editor) close(path string) {
	e.notify(methodDidClose, documentParams{TextDocument: textDocumentIdentifier{URI: uriFromPath(path)}})
}

func (e *editor) save(path string) {
	e.notify(methodDidSave, documentParams{TextDocument: textDocumentIdentifier{URI: uriFromPath(path)}})
}

func (e *editor) command(name string, arguments ...any) {
	e.t.Helper()
	e.call(methodExecuteCommand, map[string]any{"command": name, "arguments": arguments})
}

// awaitFindings waits until a list of exactly want findings is published for
// path, skipping lists published on the way there.
func (e *editor) awaitFindings(path string, want int) []diagnostic {
	e.t.Helper()
	deadline := time.After(scenarioTimeout)
	last := -1
	for {
		select {
		case published := <-e.diagnostics:
			if published.URI != uriFromPath(path) {
				continue
			}
			if last = len(published.Diagnostics); last == want {
				return published.Diagnostics
			}
		case <-deadline:
			e.t.Fatalf("%s never had %d findings published; last list had %d", path, want, last)
			return nil
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// awaitDiagnostics returns the next published list for path.
func (e *editor) awaitDiagnostics(path string) []diagnostic {
	e.t.Helper()
	deadline := time.After(scenarioTimeout)
	for {
		select {
		case published := <-e.diagnostics:
			if published.URI == uriFromPath(path) {
				return published.Diagnostics
			}
		case <-deadline:
			e.t.Fatalf("no diagnostics published for %s", path)
			return nil
		}
	}
}

func (e *editor) awaitStatus(state string) StatusParams {
	e.t.Helper()
	deadline := time.After(scenarioTimeout)
	for {
		select {
		case status := <-e.statuses:
			if status.State == state {
				return status
			}
		case <-deadline:
			e.t.Fatalf("status never became %q", state)
			return StatusParams{}
		}
	}
}

// expectQuiet fails when anything is published for path within the window.
func (e *editor) expectQuiet(path string, window time.Duration) {
	e.t.Helper()
	deadline := time.After(window)
	for {
		select {
		case published := <-e.diagnostics:
			if published.URI == uriFromPath(path) {
				e.t.Fatalf("unexpected diagnostics for %s: %+v", path, published.Diagnostics)
			}
		case <-deadline:
			return
		}
	}
}

func TestOpenPublishesFindingWithEvidence(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root})
	client.open(path, leakSource)

	findings := client.awaitDiagnostics(path)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	finding := findings[0]
	if finding.Code != "resourcelifetime/missing-release" || finding.Source != "gohawk" || finding.Severity != severityWarning {
		t.Errorf("finding identity = %q %q severity %d", finding.Code, finding.Source, finding.Severity)
	}
	if want := (position{Line: 8, Character: 11}); finding.Range.Start != want {
		t.Errorf("finding starts at %+v, want %+v", finding.Range.Start, want)
	}
	if len(finding.RelatedInformation) != 2 {
		t.Fatalf("related = %+v, want the branch and the return", finding.RelatedInformation)
	}
	for _, related := range finding.RelatedInformation {
		if related.Location.URI != uriFromPath(path) {
			t.Errorf("related location %q is not the edited file", related.Location.URI)
		}
	}
	if finding.CodeDescription == nil || finding.Data == nil || finding.Data.Help == "" {
		t.Errorf("finding lacks catalog details: %+v", finding)
	}
	if status := client.awaitStatus(stateFindings); status.Findings != 1 {
		t.Errorf("status findings = %d, want 1", status.Findings)
	}
}

func TestUnsavedFixClearsFindingAndBrokenBuildKeepsIt(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root})
	client.open(path, leakSource)
	if findings := client.awaitDiagnostics(path); len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}

	// The fix exists only in the buffer; the file on disk still leaks.
	client.change(path, fixedSource, 2)
	if findings := client.awaitDiagnostics(path); len(findings) != 0 {
		t.Fatalf("findings after unsaved fix = %+v, want none", findings)
	}
	client.awaitStatus(stateIdle)

	// Bring the finding back, then break the build: the finding must stay,
	// reported as possibly stale, and nothing new may be published.
	client.change(path, leakSource, 3)
	if findings := client.awaitDiagnostics(path); len(findings) != 1 {
		t.Fatalf("findings after reintroducing the leak = %+v, want one", findings)
	}
	client.change(path, strings.Replace(leakSource, `"io"`, `"ioo"`, 1), 4)
	status := client.awaitStatus(stateStale)
	if status.Findings != 1 || !strings.Contains(status.Text, "build failed in leak") || !strings.Contains(status.Tooltip, "stale") {
		t.Errorf("status = %+v, want one finding kept and the build failure named", status)
	}
	client.expectQuiet(path, 300*time.Millisecond)
}

func TestFilesGohawkDoesNotCheck(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource, "leak/leak_test.go": "package leak\n"})
	outside := writeModule(t, map[string]string{"other/other.go": strings.Replace(leakSource, "package leak", "package other", 1)})
	client := startEditor(t, editorOptions{workspace: root})

	// A file outside the workspace is what an editor opens to show a
	// definition. It must not be analyzed, and must not disturb the status.
	outsidePath := filepath.Join(outside, "other", "other.go")
	client.open(outsidePath, leakSource)
	client.expectQuiet(outsidePath, 500*time.Millisecond)

	client.open(filepath.Join(root, "leak", "leak_test.go"), "package leak\n")
	status := client.awaitStatus(stateSkipped)
	if !strings.Contains(status.Tooltip, "test files") {
		t.Errorf("skip reason = %q, want it to name test files", status.Tooltip)
	}
}

func TestImportersAreCheckedAfterTheEditedPackage(t *testing.T) {
	t.Parallel()
	importer := strings.Replace(leakSource, "package leak", "package app\n\nimport _ \"example.com/editor/lib\"", 1)
	root := writeModule(t, map[string]string{
		"lib/lib.go": "package lib\n\nfunc Value() int { return 1 }\n",
		"app/app.go": importer,
	})
	client := startEditor(t, editorOptions{workspace: root})
	client.open(filepath.Join(root, "lib", "lib.go"), "package lib\n\nfunc Value() int { return 1 }\n")

	// app was never opened; its finding arrives through the importer run.
	findings := client.awaitDiagnostics(filepath.Join(root, "app", "app.go"))
	if len(findings) != 1 {
		t.Fatalf("importer findings = %+v, want one", findings)
	}
}

func TestMemoryLimitStopsTheRunWithoutACleanResult(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("only Linux measures a process tree")
	}
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"memoryLimit": "1MiB", "scope": scopePackage}})
	client.open(path, leakSource)

	status := client.awaitStatus(stateStopped)
	if !strings.Contains(status.Text, "stopped") {
		t.Errorf("status text = %q, want it to say the check was stopped", status.Text)
	}
	client.expectQuiet(path, 300*time.Millisecond)
}

func TestActionsNavigateButNeverEdit(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root, clientCommands: []string{stepPathClientCommand}})
	client.open(path, leakSource)
	findings := client.awaitDiagnostics(path)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}

	raw := client.call(methodCodeAction, map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(path)},
		"range":        findings[0].Range,
		"context":      map[string]any{"diagnostics": findings},
	})
	var actions []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &actions); err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, action := range actions {
		if _, edits := action["edit"]; edits {
			t.Errorf("action %s carries a source edit", action["title"])
		}
		var invoked command
		if err := json.Unmarshal(action["command"], &invoked); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, invoked.Command)
	}
	want := []string{stepPathClientCommand, commandOpenDocumentation}
	if strings.Join(commands, ",") != strings.Join(want, ",") {
		t.Errorf("commands = %v, want %v", commands, want)
	}

	var shown hover
	hoverRaw := client.call(methodHover, positionParams{TextDocument: textDocumentIdentifier{URI: uriFromPath(path)}, Position: findings[0].Range.Start})
	if err := json.Unmarshal(hoverRaw, &shown); err != nil || !strings.Contains(shown.Contents.Value, "help for resourcelifetime/missing-release") {
		t.Errorf("hover = %s (%v), want the check's help", hoverRaw, err)
	}
}

func TestEventLogOrdersAnEditBeforeTheFindingsThatAnswerIt(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	log := &lockedBuffer{}
	client := startEditor(t, editorOptions{workspace: root, eventLog: log, settings: map[string]any{"scope": scopePackage}})
	client.open(path, leakSource)
	client.awaitDiagnostics(path)
	client.awaitStatus(stateFindings)

	var order []string
	for _, logged := range log.events(t) {
		if _, err := time.Parse(time.RFC3339Nano, logged.Time); err != nil {
			t.Errorf("event %q has time %q: %v", logged.Event, logged.Time, err)
		}
		if logged.Event != "status" {
			order = append(order, logged.Event)
		}
		if logged.Event == "run-finished" && (logged.Outcome != "completed" || logged.Findings == nil || *logged.Findings != 1) {
			t.Errorf("run-finished = %+v, want a completed run with one finding", logged)
		}
		if logged.Event == "run-finished" && (logged.ServerBytes == 0 || logged.Goroutines == 0) {
			t.Errorf("run-finished = %+v, want the server's own footprint", logged)
		}
	}
	if want := "document,run-started,run-finished,published"; strings.Join(order, ",") != want {
		t.Errorf("event order = %v, want %s", order, want)
	}
}
