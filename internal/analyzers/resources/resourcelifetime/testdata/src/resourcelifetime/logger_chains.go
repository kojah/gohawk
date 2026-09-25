package resourcelifetime

// A logger built over a file keeps the file as its sink. When a chain of
// constructors that each keep their argument ends in the process default
// logger, or in a field of an object this function did not allocate, the
// file outlives the function and its release is no longer this function's
// to prove. An early return before that handover still abandons the file,
// and so does a chain whose result is discarded.
// Real-world forms:
// https://github.com/inkdust2021/VibeGuard/blob/12a46784a7ebca95f7765178edd8344a974da849/internal/log/log.go#L42-L47
// https://github.com/1parado/grok-build-switch/blob/c1ee703bf6000abd92d8d29ca94a4ed59cb510f2/main.go#L173-L176

import (
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
)

func installDefaultSlog(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	handler := slog.NewTextHandler(io.MultiWriter(os.Stderr, file), &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
	return nil
}

func installDefaultSlogDirect(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(file, nil)))
	return nil
}

func installDefaultSlogAfterCheck(path string, fail bool) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644) // want "owned resource from os.OpenFile is not released"
	if err != nil {
		return err
	}
	if fail {
		return errors.New("not installed")
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(file, nil)))
	return nil
}

func discardSlogChain(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644) // want "owned resource from os.OpenFile is not released"
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, file), nil))
	logger.Info("started")
	return nil
}

type serverSource struct{ server *http.Server }

func (source *serverSource) current() *http.Server { return source.server }

func routeServerErrors(source *serverSource, path string) {
	server := source.current()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	server.ErrorLog = log.New(file, "http: ", log.LstdFlags)
}

func routeLocalServerErrors(path string) {
	server := &http.Server{}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644) // want "owned resource from os.OpenFile is not released"
	if err != nil {
		return
	}
	server.ErrorLog = log.New(file, "http: ", log.LstdFlags)
	_ = server
}

var serverErrors *log.Logger

func routeErrorsToPackageLogger(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	serverErrors = log.New(file, "http: ", log.LstdFlags)
	return nil
}
