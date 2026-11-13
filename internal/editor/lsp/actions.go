package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// What a user can do with a finding: read its help, follow its reported
// path, and open its documentation. Every action navigates. None edits
// source, and there is no action that hides a finding.

const (
	codeActionKind           = "quickfix"
	commandCheckPackage      = "gohawk.server.checkPackage"
	commandCheckWorkspace    = "gohawk.server.checkWorkspace"
	commandClearResults      = "gohawk.server.clearResults"
	commandOpenDocumentation = "gohawk.server.openDocumentation"
)

func serverCommands() []string {
	return []string{commandCheckPackage, commandCheckWorkspace, commandClearResults, commandOpenDocumentation}
}

// findingTarget names one finding. The client command receives it and looks
// the finding up among the diagnostics the editor already holds.
type findingTarget struct {
	URI       string `json:"uri"`
	Line      int    `json:"line"` // one-based, as the terminal prints it
	Character int    `json:"character"`
	Analyzer  string `json:"analyzer"`
	Check     string `json:"check"`
}

func (s *Server) hover(raw json.RawMessage) *hover {
	var params positionParams
	if !decode(raw, &params) {
		return nil
	}
	path, ok := pathFromURI(params.TextDocument.URI)
	if !ok {
		return nil
	}
	var sections []string
	var covered *textRange
	for _, finding := range s.results.forFile(path) {
		if !contains(finding.Range, params.Position) || finding.Data == nil || finding.Data.Help == "" {
			continue
		}
		section := "**gohawk help:** " + finding.Data.Help
		if finding.CodeDescription != nil {
			section += fmt.Sprintf("\n\n[About %s](%s)", finding.Code, finding.CodeDescription.Href)
		}
		sections = append(sections, section)
		covered = &finding.Range
	}
	if len(sections) == 0 {
		return nil
	}
	return &hover{Contents: markupContent{Format: "markdown", Value: strings.Join(sections, "\n\n---\n\n")}, Range: covered}
}

func contains(span textRange, point position) bool {
	afterStart := point.Line > span.Start.Line || point.Line == span.Start.Line && point.Character >= span.Start.Character
	beforeEnd := point.Line < span.End.Line || point.Line == span.End.Line && point.Character <= span.End.Character
	return afterStart && beforeEnd
}

func (s *Server) codeActions(raw json.RawMessage) []codeAction {
	actions := []codeAction{}
	var params codeActionParams
	if !decode(raw, &params) {
		return actions
	}
	for _, finding := range params.Context.Diagnostics {
		if finding.Source != diagnosticSource {
			continue
		}
		actions = append(actions, s.actionsFor(params.TextDocument.URI, finding)...)
	}
	return actions
}

func (s *Server) actionsFor(uri string, finding diagnostic) []codeAction {
	var actions []codeAction
	add := func(title, name string, arguments ...any) {
		actions = append(actions, codeAction{
			Title: title, Category: codeActionKind, Diagnostics: []diagnostic{finding},
			Command: &command{Title: title, Command: name, Arguments: arguments},
		})
	}
	// The client implements this one: stepping needs editor decorations.
	// Other editors are not offered it.
	if finding.Data != nil {
		target := findingTarget{
			URI: uri, Line: finding.Range.Start.Line + 1, Character: finding.Range.Start.Character,
			Analyzer: finding.Data.Analyzer, Check: finding.Code,
		}
		if s.clientCommands[stepPathClientCommand] && len(finding.RelatedInformation) > 0 {
			add("gohawk: Step through the reported path", stepPathClientCommand, target)
		}
	}
	if finding.CodeDescription != nil && isWebAddress(finding.CodeDescription.Href) {
		add("gohawk: Open documentation for "+finding.Code, commandOpenDocumentation, finding.CodeDescription.Href)
	}
	return actions
}

func (s *Server) executeCommand(ctx context.Context, incoming *message) {
	var params executeCommandParams
	if !decode(incoming.Params, &params) {
		_ = s.connection.replyError(incoming.ID, errorInvalidParams, "invalid command parameters")
		return
	}
	switch params.Command {
	case commandCheckPackage:
		if path, ok := firstPathArgument(params.Arguments); ok {
			s.requestCheck(ctx, path, 0)
		}
	case commandCheckWorkspace:
		s.checkWorkspaces(ctx, params.Arguments)
	case commandClearResults:
		s.publish(s.results.clear())
		s.scheduler.last = runSummary{}
		s.publishStatus()
	case commandOpenDocumentation:
		var target string
		if len(params.Arguments) == 0 || !decode(params.Arguments[0], &target) || !isWebAddress(target) {
			_ = s.connection.replyError(incoming.ID, errorInvalidParams, "documentation is opened only from an http or https address")
			return
		}
		s.nextRequestID++
		_ = s.connection.request(s.nextRequestID, methodShowDocument, showDocumentParams{URI: target, External: true})
	default:
		_ = s.connection.replyError(incoming.ID, errorMethodNotFound, "unknown command "+params.Command)
		return
	}
	_ = s.connection.reply(incoming.ID, nil)
}

// checkWorkspaces queues a whole-module run for the module of the named file,
// or for the module of every open file when none is named.
func (s *Server) checkWorkspaces(ctx context.Context, arguments []json.RawMessage) {
	var paths []string
	if path, ok := firstPathArgument(arguments); ok {
		paths = append(paths, path)
	} else {
		for path := range s.documents.byPath {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		if root, ok := moduleRoot(filepath.Dir(path)); ok {
			s.scheduler.workspaces[root] = true
		}
	}
	s.pump(ctx)
}

func firstPathArgument(arguments []json.RawMessage) (string, bool) {
	var uri string
	if len(arguments) == 0 || !decode(arguments[0], &uri) {
		return "", false
	}
	return pathFromURI(uri)
}

// isWebAddress accepts only what a documentation link can be. The client is
// asked to open the address outside the editor, so nothing else is passed on.
func isWebAddress(target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}
