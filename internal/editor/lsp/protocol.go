package lsp

import "encoding/json"

// The Language Server Protocol subset the gohawk server speaks. Only the
// messages the server sends or handles are declared; fields the server never
// reads are omitted so an unknown client capability cannot change behavior.

const (
	methodInitialize             = "initialize"
	methodInitialized            = "initialized"
	methodShutdown               = "shutdown"
	methodExit                   = "exit"
	methodDidOpen                = "textDocument/didOpen"
	methodDidChange              = "textDocument/didChange"
	methodDidSave                = "textDocument/didSave"
	methodDidClose               = "textDocument/didClose"
	methodHover                  = "textDocument/hover"
	methodCodeAction             = "textDocument/codeAction"
	methodExecuteCommand         = "workspace/executeCommand"
	methodDidChangeConfiguration = "workspace/didChangeConfiguration"
	methodDidChangeWatchedFiles  = "workspace/didChangeWatchedFiles"
	methodPublishDiagnostics     = "textDocument/publishDiagnostics"
	methodLogMessage             = "window/logMessage"
	methodShowDocument           = "window/showDocument"
	methodStatus                 = "gohawk/status"
)

const (
	errorParse          = -32700
	errorInvalidRequest = -32600
	errorNotInitialized = -32002
	errorMethodNotFound = -32601
	errorInvalidParams  = -32602
	errorRequestFailed  = -32803
)

const (
	severityWarning     = 2
	severityInformation = 3
)

const (
	messageError = 1
	messageInfo  = 3
)

const textDocumentSyncFull = 1

type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *responseError   `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type textRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type location struct {
	URI   string    `json:"uri"`
	Range textRange `json:"range"`
}

type relatedInformation struct {
	Location location `json:"location"`
	Message  string   `json:"message"`
}

type codeDescription struct {
	Href string `json:"href"`
}

// diagnosticData travels with a diagnostic so a later code action request
// can name the finding without the server searching for it again.
type diagnosticData struct {
	Analyzer string `json:"analyzer"`
	Help     string `json:"help,omitempty"`
	URI      string `json:"uri"`
}

type diagnostic struct {
	Range              textRange            `json:"range"`
	Severity           int                  `json:"severity"`
	Code               string               `json:"code,omitempty"`
	CodeDescription    *codeDescription     `json:"codeDescription,omitempty"`
	Source             string               `json:"source"`
	Message            string               `json:"message"`
	RelatedInformation []relatedInformation `json:"relatedInformation,omitempty"`
	Data               *diagnosticData      `json:"data,omitempty"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type textDocumentItem struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
	Text    string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Text string `json:"text"`
}

type didChangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	ContentChanges []contentChange `json:"contentChanges"`
}

// fileEvent is one change the editor saw on disk. Change is 1 for created,
// 2 for changed, and 3 for deleted.
type fileEvent struct {
	URI    string `json:"uri"`
	Change int    `json:"type"`
}

type watchedFilesParams struct {
	Changes []fileEvent `json:"changes"`
}

const fileDeleted = 3

type documentParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type positionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range        textRange              `json:"range"`
	Context      struct {
		Diagnostics []diagnostic `json:"diagnostics"`
	} `json:"context"`
}

type command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

type codeAction struct {
	Title string `json:"title"`
	// Category is the protocol's action kind, such as quickfix.
	Category    string       `json:"kind"`
	Diagnostics []diagnostic `json:"diagnostics,omitempty"`
	Command     *command     `json:"command,omitempty"`
}

type executeCommandParams struct {
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments"`
}

type markupContent struct {
	// Format is the protocol's markup kind: markdown or plaintext.
	Format string `json:"kind"`
	Value  string `json:"value"`
}

type hover struct {
	Contents markupContent `json:"contents"`
	Range    *textRange    `json:"range,omitempty"`
}

type logMessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

type showDocumentParams struct {
	URI      string `json:"uri"`
	External bool   `json:"external"`
}

type workspaceFolder struct {
	URI string `json:"uri"`
}

type initializeParams struct {
	RootURI               string            `json:"rootUri"`
	WorkspaceFolders      []workspaceFolder `json:"workspaceFolders"`
	InitializationOptions json.RawMessage   `json:"initializationOptions"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type serverCapabilities struct {
	TextDocumentSync       textDocumentSyncOptions `json:"textDocumentSync"`
	HoverProvider          bool                    `json:"hoverProvider"`
	CodeActionProvider     codeActionOptions       `json:"codeActionProvider"`
	ExecuteCommandProvider executeCommandOptions   `json:"executeCommandProvider"`
}

type textDocumentSyncOptions struct {
	OpenClose bool        `json:"openClose"`
	Change    int         `json:"change"`
	Save      saveOptions `json:"save"`
}

type saveOptions struct {
	IncludeText bool `json:"includeText"`
}

type codeActionOptions struct {
	CodeActionKinds []string `json:"codeActionKinds"`
}

type executeCommandOptions struct {
	Commands []string `json:"commands"`
}

// StatusParams is the payload of the gohawk/status notification. The server
// composes the text so every client shows the same wording.
type StatusParams struct {
	// State is one of idle, findings, checking, queued, stale, stopped,
	// error, or skipped.
	State    string `json:"state"`
	Text     string `json:"text"`
	Tooltip  string `json:"tooltip,omitempty"`
	Findings int    `json:"findings"`
}
