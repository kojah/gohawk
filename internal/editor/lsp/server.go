// Package lsp serves gohawk findings to editors over the Language Server
// Protocol.
//
// The server holds no analysis state. It tracks open buffers, decides when a
// package needs checking, and starts `go vet` with gohawk as its tool for each
// check; the child process owns all analysis memory and exits when done. The
// server only schedules and presents: it never changes which diagnostics
// exist, and it offers no source edits.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Options configures a server.
type Options struct {
	// Executable is the gohawk binary go vet runs as its tool.
	Executable string
	Version    string
	Catalog    Catalog
	// EventLog, when set, receives one JSON line per scheduling event.
	EventLog io.Writer
}

// Server is one editor session. All state is owned by the goroutine running
// Serve; other goroutines hand their results back through the events channel.
type Server struct {
	options        Options
	eventLog       *eventLog
	connection     *connection
	runner         *runner
	settings       settings
	clientCommands map[string]bool
	// workspaceRoots are the folders the editor has open. Only files inside
	// them are checked: editors also open dependency and standard library
	// files to show a definition, and those are not the user's code.
	workspaceRoots []string
	spelling       *pathSpelling
	documents      *documentStore
	results        *resultStore
	events         chan func()
	done           chan struct{}
	// runs counts check goroutines, so the server can wait for their child
	// processes to be stopped before it returns.
	runs          sync.WaitGroup
	scheduler     scheduler
	lastStatus    StatusParams
	nextRequestID int
	shutdown      bool
	initialized   bool
	// stopExplain cancels the one explanation in flight, or is nil.
	stopExplain context.CancelFunc
}

// Serve runs the server until the client sends exit or closes the stream.
func Serve(ctx context.Context, input io.Reader, output io.Writer, options Options) error {
	return serve(ctx, newConnection(input, output), options)
}

func serve(ctx context.Context, client *connection, options Options) error {
	removeAbandonedDirectories()
	temporary, err := newTemporaryDirectory()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	if options.Catalog == nil {
		options.Catalog = func(string) CheckInfo { return CheckInfo{} }
	}
	server := &Server{
		options:        options,
		eventLog:       &eventLog{writer: options.EventLog},
		connection:     client,
		runner:         &runner{executable: options.Executable, temporary: temporary},
		settings:       defaultSettings(),
		clientCommands: make(map[string]bool),
		documents:      newDocumentStore(),
		results:        newResultStore(),
		events:         make(chan func(), 64),
		done:           make(chan struct{}),
	}
	server.scheduler = newScheduler()
	return server.loop(ctx)
}

func (s *Server) loop(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer s.connection.close()
	defer close(s.done)
	defer s.stopRuns()
	incoming := make(chan *message)
	readErr := make(chan error, 1)
	go s.readMessages(ctx, incoming, readErr)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.connection.stalled:
			return errStalled
		case err := <-readErr:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case event := <-s.events:
			event()
		case next := <-incoming:
			if exit := s.handle(ctx, next); exit {
				return nil
			}
		}
	}
}

// readMessages feeds the event loop. A body that is not a valid message is
// refused and reading continues; an error that loses the stream ends it.
func (s *Server) readMessages(ctx context.Context, incoming chan<- *message, failed chan<- error) {
	for {
		next, err := s.connection.read()
		if errors.Is(err, errMalformedMessage) {
			_ = s.connection.replyError(nil, errorParse, err.Error())
			continue
		}
		if err != nil {
			failed <- err
			return
		}
		select {
		case incoming <- next:
		case <-ctx.Done():
			return
		}
	}
}

// stopRuns cancels every check and waits for their processes to be stopped.
// Returning earlier would let the server exit while a child still runs. The
// wait is bounded: a child that ignores the kill must not hold the editor.
func (s *Server) stopRuns() {
	s.cancelAllRuns()
	stopped := make(chan struct{})
	go func() {
		s.runs.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
	}
}

// post hands work back to the serving goroutine. It gives up when the server
// has stopped, so a late run completion cannot block forever.
func (s *Server) post(event func()) {
	select {
	case s.events <- event:
	case <-s.done:
	}
}

func (s *Server) handle(ctx context.Context, incoming *message) (exit bool) {
	isReply := incoming.Method == "" && (incoming.Result != nil || incoming.Error != nil)
	switch {
	case isReply:
		return false // the answer to our request to show a document
	case incoming.Method == "":
		_ = s.connection.replyError(incoming.ID, errorInvalidRequest, "the message names no method")
		return false
	case incoming.ID == nil:
		return s.handleNotification(ctx, incoming)
	}
	s.handleRequest(ctx, incoming)
	return false
}

func (s *Server) handleNotification(ctx context.Context, incoming *message) (exit bool) {
	if incoming.Method == methodExit {
		return true
	}
	// Before initialize the server does not know the workspace, so it cannot
	// tell the user's files from any other file on disk.
	if !s.initialized {
		return false
	}
	switch incoming.Method {
	case methodDidOpen:
		var params didOpenParams
		if decode(incoming.Params, &params) {
			s.didOpen(ctx, params)
		}
	case methodDidChange:
		var params didChangeParams
		if decode(incoming.Params, &params) {
			s.didChange(ctx, params)
		}
	case methodDidSave:
		var params documentParams
		if decode(incoming.Params, &params) {
			s.didSave(ctx, params)
		}
	case methodDidClose:
		var params documentParams
		if decode(incoming.Params, &params) {
			s.didClose(ctx, params)
		}
	case methodDidChangeConfiguration:
		var params configurationParams
		if decode(incoming.Params, &params) && params.Settings.Gohawk != nil {
			s.applySettings(ctx, params.Settings.Gohawk.normalized())
		}
	case methodDidChangeWatchedFiles:
		var params watchedFilesParams
		if decode(incoming.Params, &params) {
			s.didChangeWatchedFiles(ctx, params)
		}
	}
	return false
}

func (s *Server) handleRequest(ctx context.Context, incoming *message) {
	var err error
	if !s.initialized && incoming.Method != methodInitialize && incoming.Method != methodShutdown {
		_ = s.connection.replyError(incoming.ID, errorNotInitialized, "initialize has not been sent")
		return
	}
	switch incoming.Method {
	case methodInitialize:
		err = s.connection.reply(incoming.ID, s.initialize(incoming.Params))
	case methodShutdown:
		s.shutdown = true
		s.cancelAllRuns()
		err = s.connection.reply(incoming.ID, nil)
	case methodHover:
		err = s.connection.reply(incoming.ID, s.hover(incoming.Params))
	case methodCodeAction:
		err = s.connection.reply(incoming.ID, s.codeActions(incoming.Params))
	case methodExecuteCommand:
		s.executeCommand(ctx, incoming)
	default:
		err = s.connection.replyError(incoming.ID, errorMethodNotFound, "unsupported method "+incoming.Method)
	}
	if err != nil {
		s.logf("reply to %s: %v", incoming.Method, err)
	}
}

func (s *Server) initialize(raw json.RawMessage) initializeResult {
	var params initializeParams
	if decode(raw, &params) {
		s.workspaceRoots = workspaceRoots(params)
	}
	s.spelling = newPathSpelling(s.workspaceRoots)
	s.initialized = true
	if len(params.InitializationOptions) > 0 {
		var options initializationOptions
		options.Settings = s.settings
		if decode(params.InitializationOptions, &options) {
			s.settings = options.Settings.normalized()
			for _, name := range options.ClientCommands {
				s.clientCommands[name] = true
			}
		}
	}
	return initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync:       textDocumentSyncOptions{OpenClose: true, Change: textDocumentSyncFull},
			HoverProvider:          true,
			CodeActionProvider:     codeActionOptions{CodeActionKinds: []string{codeActionKind}},
			ExecuteCommandProvider: executeCommandOptions{Commands: serverCommands()},
		},
		ServerInfo: serverInfo{Name: "gohawk", Version: s.options.Version},
	}
}

func (s *Server) didOpen(ctx context.Context, params didOpenParams) {
	path, ok := pathFromURI(params.TextDocument.URI)
	if !ok {
		return
	}
	s.documents.open(path, params.TextDocument.Text, params.TextDocument.Version)
	s.eventLog.record(event{Event: "document", Detail: "open", Path: path, Version: params.TextDocument.Version})
	if s.settings.Run != runManual {
		s.requestCheck(ctx, path, s.settings.debounce())
	}
}

func (s *Server) didChange(ctx context.Context, params didChangeParams) {
	path, ok := pathFromURI(params.TextDocument.URI)
	if !ok || len(params.ContentChanges) == 0 {
		return
	}
	text := params.ContentChanges[len(params.ContentChanges)-1].Text
	if !s.documents.change(path, text, params.TextDocument.Version) {
		return
	}
	s.eventLog.record(event{Event: "document", Detail: "change", Path: path, Version: params.TextDocument.Version})
	// Only when checks follow typing do findings describe the buffer. In the
	// other modes they describe the saved file, which an edit does not change,
	// so a run in flight stays valid.
	if s.settings.Run == runOnPause {
		s.requestCheck(ctx, path, s.settings.debounce())
	}
}

// didClose handles a buffer the editor dropped. If it held unsaved text the
// findings shown were computed from text that is gone, so the package is
// checked again from disk.
func (s *Server) didClose(ctx context.Context, params documentParams) {
	path, ok := pathFromURI(params.TextDocument.URI)
	if !ok {
		return
	}
	if s.documents.close(path) && s.settings.Run == runOnPause && s.skipReason(path) == "" {
		s.requestDirectory(ctx, filepath.Dir(path), 0)
	}
}

// applySettings installs new settings. Turning experimental checks off
// removes their findings at once; turning them on or off re-checks the open
// packages, because the stored findings were computed under the old choice.
func (s *Server) applySettings(ctx context.Context, next settings) {
	experimentalChanged := next.Experimental != s.settings.Experimental
	s.settings = next
	if !experimentalChanged {
		return
	}
	if !next.Experimental {
		s.publish(s.results.dropWhere(func(finding diagnostic) bool { return finding.Severity == severityInformation }))
	}
	if next.Run == runManual {
		s.publishStatus()
		return
	}
	directories := make(map[string]bool)
	for path := range s.documents.byPath {
		if s.skipReason(path) == "" {
			directories[filepath.Dir(path)] = true
		}
	}
	for _, directory := range sortedKeys(directories) {
		s.requestDirectory(ctx, directory, 0)
	}
	s.publishStatus()
}

// didChangeWatchedFiles handles changes the editor saw on disk: a deleted
// file or directory loses its findings at once, and a package whose files
// changed outside an open buffer is checked again.
func (s *Server) didChangeWatchedFiles(ctx context.Context, params watchedFilesParams) {
	directories := make(map[string]bool)
	for _, change := range params.Changes {
		path, ok := pathFromURI(change.URI)
		if !ok || !s.insideWorkspace(path) {
			continue
		}
		if change.Change == fileDeleted {
			s.publish(s.results.dropUnder(path))
		}
		// A save of an open buffer arrives as didSave; checking again here
		// would only repeat it.
		if s.documents.isOpen(path) || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			continue
		}
		if info, err := os.Stat(filepath.Dir(path)); err == nil && info.IsDir() {
			directories[filepath.Dir(path)] = true
		}
	}
	if s.settings.Run != runManual {
		for _, directory := range sortedKeys(directories) {
			if _, ok := moduleRoot(directory); ok {
				s.requestDirectory(ctx, directory, s.settings.debounce())
			}
		}
	}
	s.publishStatus()
}

func (s *Server) didSave(ctx context.Context, params documentParams) {
	path, ok := pathFromURI(params.TextDocument.URI)
	if !ok {
		return
	}
	s.documents.saved(path)
	s.eventLog.record(event{Event: "document", Detail: "save", Path: path})
	if s.settings.Run == runManual {
		return
	}
	s.requestCheck(ctx, path, 0)
	if root, found := moduleRoot(filepath.Dir(path)); found && s.settings.Scope == scopeWorkspaceOnSave {
		s.scheduler.workspaces[root] = true
	}
}

func workspaceRoots(params initializeParams) []string {
	folders := params.WorkspaceFolders
	if len(folders) == 0 && params.RootURI != "" {
		folders = []workspaceFolder{{URI: params.RootURI}}
	}
	var roots []string
	for _, folder := range folders {
		if path, ok := pathFromURI(folder.URI); ok {
			roots = append(roots, path)
		}
	}
	return roots
}

// skipReason explains why a file gets no gohawk findings, or returns the
// empty string when the file's package should be checked.
func (s *Server) skipReason(path string) string {
	switch {
	case filepath.Ext(path) != ".go":
		return "not a Go file"
	case strings.HasSuffix(path, "_test.go"):
		return "gohawk does not analyze test files"
	case !s.insideWorkspace(path):
		return "the file is outside the open workspace"
	}
	if _, ok := moduleRoot(filepath.Dir(path)); !ok {
		return "the file is outside a Go module"
	}
	return ""
}

// insideWorkspace accepts every file when the editor named no folder, which
// is how a single file opened on its own arrives.
func (s *Server) insideWorkspace(path string) bool {
	if len(s.workspaceRoots) == 0 {
		return true
	}
	for _, root := range s.workspaceRoots {
		if insideDirectory(root, path) {
			return true
		}
	}
	return false
}

func decode(raw json.RawMessage, target any) bool {
	return len(raw) > 0 && json.Unmarshal(raw, target) == nil
}

func (s *Server) logf(format string, arguments ...any) {
	text := time.Now().Format("15:04:05") + " " + fmt.Sprintf(format, arguments...)
	_ = s.connection.notify(methodLogMessage, logMessageParams{Type: messageInfo, Message: text})
}
