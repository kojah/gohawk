package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A client can be buggy, hostile, or simply gone. These scenarios check that
// one bad message costs one error reply, that nothing a client sends makes
// the server unbounded or unstoppable, and that it does nothing before it
// has been told the workspace.

type rawClient struct {
	t        *testing.T
	toServer io.WriteCloser
	fromHost *connection
	served   chan error
}

func startRaw(t *testing.T) *rawClient {
	t.Helper()
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	client := &rawClient{t: t, toServer: fromClient, fromHost: newConnection(toClient, io.Discard), served: make(chan error, 1)}
	go func() {
		client.served <- Serve(t.Context(), toServer, fromServer, Options{Executable: "gohawk-not-needed"})
		_ = fromServer.Close()
	}()
	t.Cleanup(func() { _ = fromClient.Close() })
	return client
}

func (c *rawClient) send(body string) {
	c.t.Helper()
	if _, err := fmt.Fprintf(c.toServer, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		c.t.Fatal(err)
	}
}

// reply returns the next response, skipping notifications.
func (c *rawClient) reply() *message {
	c.t.Helper()
	for {
		next, err := c.fromHost.read()
		if err != nil {
			c.t.Fatalf("read reply: %v", err)
		}
		if next.Method == "" {
			return next
		}
	}
}

func (c *rawClient) initialize(root string) {
	c.t.Helper()
	c.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q}}`, uriFromPath(root)))
	if reply := c.reply(); reply.Error != nil {
		c.t.Fatalf("initialize: %s", reply.Error.Message)
	}
}

// commandRequest renders an executeCommand request with one raw JSON argument.
func commandRequest(id int, name, argument string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"workspace/executeCommand","params":{"command":%q,"arguments":[%s]}}`, id, name, argument)
}

func TestOneBadMessageCostsOneErrorReply(t *testing.T) {
	t.Parallel()
	client := startRaw(t)
	client.initialize(t.TempDir())
	for _, test := range []struct {
		body string
		code int
	}{
		{`hello`, errorParse},
		{`[{"jsonrpc":"2.0","id":2,"method":"shutdown"}]`, errorParse},
		{`5`, errorParse},
		{`{"jsonrpc":"2.0","id":3,"method":5}`, errorParse},
		{`{"jsonrpc":"2.0","id":4,"method":""}`, errorInvalidRequest},
		{`{"jsonrpc":"2.0","id":5,"method":"no/such"}`, errorMethodNotFound},
		{commandRequest(6, commandOpenDocumentation, `"file:///etc/passwd"`), errorInvalidParams},
		{commandRequest(7, commandExplain, `{"uri":"file:///etc/x.go","line":1,"analyzer":"-h"}`), errorInvalidParams},
	} {
		client.send(test.body)
		reply := client.reply()
		if reply.Error == nil || reply.Error.Code != test.code {
			t.Errorf("%s: reply = %+v, want error %d", test.body, reply.Error, test.code)
		}
	}
	// The server is still serving.
	client.send(`{"jsonrpc":"2.0","id":9,"method":"shutdown"}`)
	if reply := client.reply(); reply.Error != nil || string(*reply.ID) != "9" {
		t.Errorf("shutdown after bad messages: %+v", reply)
	}
}

func TestNothingIsCheckedBeforeInitialize(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	client := startRaw(t)
	document := textDocumentItem{URI: uriFromPath(filepath.Join(root, "leak", "leak.go")), Version: 1, Text: leakSource}
	open, err := json.Marshal(didOpenParams{TextDocument: document})
	if err != nil {
		t.Fatal(err)
	}
	// Without a workspace the server cannot tell the user's files from any
	// other file, so it must not start a check. A check would fail loudly
	// here: the configured binary does not exist.
	client.send(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":` + string(open) + `}`)
	client.send(`{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{}}`)
	if reply := client.reply(); reply.Error == nil || reply.Error.Code != errorNotInitialized {
		t.Errorf("request before initialize: %+v, want not-initialized", reply.Error)
	}
}

func TestAHeaderWithoutEndStopsTheServerInsteadOfGrowing(t *testing.T) {
	t.Parallel()
	client := startRaw(t)
	go func() {
		// Far more than the header limit, and never a newline.
		_, _ = client.toServer.Write(bytes.Repeat([]byte("A"), 4*maxHeaderBytes))
	}()
	select {
	case err := <-client.served:
		if err == nil || !strings.Contains(err.Error(), "header line longer") {
			t.Errorf("server ended with %v, want a header length error", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the server kept reading an endless header")
	}
}

// blockedWriter is a client that never reads.
type blockedWriter struct{ release chan struct{} }

func (w blockedWriter) Write(data []byte) (int, error) {
	<-w.release
	return len(data), nil
}

func TestAClientThatStopsReadingCannotHoldTheServer(t *testing.T) {
	t.Parallel()
	toServer, fromClient := io.Pipe()
	stuck := blockedWriter{release: make(chan struct{})}
	t.Cleanup(func() { close(stuck.release); _ = fromClient.Close() })
	client := newConnection(toServer, stuck)
	client.patience = 200 * time.Millisecond
	served := make(chan error, 1)
	go func() { served <- serve(t.Context(), client, Options{Executable: "gohawk-not-needed"}) }()
	go func() {
		// Every request earns a reply nobody reads; the queue fills.
		for id := range 4 * outgoingQueue {
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"no/such"}`, id)
			if _, err := fmt.Fprintf(fromClient, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
				return
			}
		}
	}()
	select {
	case err := <-served:
		if !errors.Is(err, errStalled) {
			t.Errorf("server ended with %v, want the stalled-client error", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the server is stuck writing to a client that does not read")
	}
}

func TestSettingsThatWouldBreakEveryCheckAreRefused(t *testing.T) {
	t.Parallel()
	if _, ok := parseByteSize("9999999TiB"); ok {
		t.Error("a memory limit that overflows was accepted")
	}
	if size, ok := parseByteSize("8388607TiB"); !ok || size <= 0 {
		t.Errorf("the largest representable limit = %d, %t", size, ok)
	}
	huge := settings{MaxParallel: 1_000_000_000, TimeoutSecs: math.MaxInt32, MemoryLimit: "9999999TiB"}.normalized()
	if huge.MaxParallel > max(runtime.NumCPU(), defaultMaxParallel) {
		t.Errorf("maxParallel = %d, want it bounded by the machine", huge.MaxParallel)
	}
	if huge.timeout() != maxTimeoutSecs*time.Second || huge.timeout() <= 0 {
		t.Errorf("timeout = %s, want it capped", huge.timeout())
	}
	if huge.MemoryLimit != defaultMemoryLimit {
		t.Errorf("memory limit = %q, want the default", huge.MemoryLimit)
	}
	long := 3_600_000_000
	if (settings{DebounceMS: &long}).debounce() != maxDebounce {
		t.Error("the pause delay was not capped")
	}
}

// A package whose go.mod is deleted during its check has no importers to
// find. Leaving it queued once started an empty run in an endless loop.
func TestPackagesWithoutAModuleAreDroppedFromTheImporterQueue(t *testing.T) {
	t.Parallel()
	server := &Server{settings: defaultSettings(), scheduler: newScheduler()}
	server.scheduler.ripple[t.TempDir()] = true
	if root, directories, ok := server.nextRipple(); ok {
		t.Errorf("a run was scheduled for root %q, directories %v", root, directories)
	}
	if len(server.scheduler.ripple) != 0 {
		t.Errorf("the importer queue still holds %v", server.scheduler.ripple)
	}
}

func TestOnlyOneExplanationRunsAtATime(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"run": runManual}})
	explain := func(id int) {
		raw := json.RawMessage(strconv.Itoa(id))
		params := mustJSON(t, map[string]any{"command": commandExplain, "arguments": []any{
			findingTarget{URI: uriFromPath(path), Line: 9, Analyzer: "resourcelifetime", Check: "resourcelifetime/missing-release"},
		}})
		if err := client.connection.write(&message{ID: &raw, Method: methodExecuteCommand, Params: params}); err != nil {
			t.Fatal(err)
		}
	}
	explain(101)
	explain(102)
	var explained, refused int
	for range 2 {
		select {
		case reply := <-client.replies:
			switch {
			case reply.Error != nil && strings.Contains(reply.Error.Message, "another explanation"):
				refused++
			case reply.Error == nil && strings.Contains(string(reply.Result), "gohawk dump trace"):
				explained++
			default:
				t.Errorf("unexpected reply: %+v %s", reply.Error, reply.Result)
			}
		case <-time.After(scenarioTimeout):
			t.Fatal("an explain request got no reply")
		}
	}
	if explained != 1 || refused != 1 {
		t.Errorf("explained %d, refused %d; want one of each", explained, refused)
	}
}
