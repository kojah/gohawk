package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/editor/lsp"
)

// The `gohawk lsp` command serves findings to an editor over standard input
// and output. The command boundary supplies what the server must not own:
// the path of this binary, which go vet runs as its tool, and the catalog
// facts that explain a check.

func runLanguageServer(arguments []string, runtime cliRuntime) error {
	flags := flag.NewFlagSet("gohawk lsp", flag.ContinueOnError)
	flags.SetOutput(runtime.errorsOutput)
	flags.Usage = func() {
		writeLine(runtime.errorsOutput, "usage: gohawk lsp [-event-log FILE]")
		writeLine(runtime.errorsOutput, "Serves gohawk findings over the Language Server Protocol on standard input and output.")
		flags.PrintDefaults()
	}
	eventLogPath := flags.String("event-log", "", "append one JSON line per scheduling event to `FILE`, for measuring latency")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		flags.Usage()
		return errors.New("lsp takes no arguments")
	}
	executable, err := runtime.executable()
	if err != nil {
		return err
	}
	var eventLog io.Writer
	if *eventLogPath != "" {
		file, err := os.OpenFile(*eventLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		eventLog = file
	}
	// Editors stop a server with either signal. Both must end it cleanly,
	// because only a clean end stops the checks it started.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	info, ok := debug.ReadBuildInfo()
	return lsp.Serve(ctx, io.Reader(os.Stdin), runtime.output, lsp.Options{
		Executable: executable,
		Version:    humanVersion(buildVersion, info, ok),
		Catalog:    editorCatalog(),
		EventLog:   eventLog,
	})
}

// editorCatalog answers the server's questions about a check from the same
// catalog the terminal renderer uses, so help text and documentation links
// agree between the two.
func editorCatalog() lsp.Catalog {
	pages := make(map[string]string)
	for _, group := range gohawk.AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			pages[analyzer.Name] = gohawk.AnalyzerDocumentationURL(group, analyzer.Name)
		}
	}
	checks := make(map[string]lsp.CheckInfo)
	for _, info := range gohawk.AnalyzerMetadata() {
		for _, check := range info.Checks {
			analyzer, _, _ := strings.Cut(string(check.ID), "/")
			checks[string(check.ID)] = lsp.CheckInfo{
				Help:             check.Help,
				DocumentationURL: pages[analyzer],
				Experimental:     check.Tier == gohawk.CheckTierExperimental,
			}
		}
	}
	return func(check string) lsp.CheckInfo { return checks[check] }
}

// EditorServerGone returns a channel that is closed when the editor server
// that started this process no longer exists, or nil when this process was
// not started by one. A server that is killed outright cannot stop its
// checks; this lets an analysis in flight stop itself instead of running to
// the end with nobody to read the result. The caller decides what to do.
func EditorServerGone() <-chan struct{} {
	pid, err := strconv.Atoi(os.Getenv(lsp.ServerProcessVariable))
	if err != nil || pid <= 0 {
		return nil
	}
	gone := make(chan struct{})
	go func() {
		for lsp.ProcessAlive(pid) {
			time.Sleep(2 * time.Second)
		}
		close(gone)
	}()
	return gone
}
