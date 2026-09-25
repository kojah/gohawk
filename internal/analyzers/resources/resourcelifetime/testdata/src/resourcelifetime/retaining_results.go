package resourcelifetime

import (
	"log/slog"
	"os"

	"resourcedep"
)

// This file covers retaining results: a constructor that opens a resource
// and returns a wrapper proven to hold it, such as a logger over a file. No
// method of the wrapper releases the resource, so the obligation moves to the
// caller, which settles it by keeping the wrapper, handing it on, or
// returning it, and leaks the resource by dropping it.
//
// Known gap: a constructor that is not exported has no summary, so its
// wrapper return is only an uncertain boundary and its callers owe nothing.
// Neither side reports that private helper.

func NewFileLogger(path string) (*slog.Logger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewTextHandler(file, nil)), nil
}

func OpenFileView(path string) (*resourcedep.View, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return resourcedep.NewView(file), nil
}

func installFileLogger(path string) error {
	logger, err := NewFileLogger(path)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	return nil
}

func forwardFileLogger(path string) (*slog.Logger, error) {
	logger, err := NewFileLogger(path)
	if err != nil {
		return nil, err
	}
	return logger, nil
}

func useFileLogger(path string, run func(*slog.Logger)) error {
	logger, err := NewFileLogger(path)
	if err != nil {
		return err
	}
	run(logger)
	return nil
}

func dropFileLogger(path string) error {
	logger, err := NewFileLogger(path) // want "resource held by the result of resourcelifetime.NewFileLogger is dropped on some return path"
	if err != nil {
		return err
	}
	_ = logger
	return nil
}

func dropFileView(path string) error {
	view, err := OpenFileView(path) // want "resource held by the result of resourcelifetime.OpenFileView is dropped on some return path"
	if err != nil {
		return err
	}
	_ = view
	return nil
}

// Using the file before wrapping it is not a fresh handover, so the returned
// logger is only an uncertain boundary and no caller inherits the file.
func NewAnnouncedLogger(path string) (*slog.Logger, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	_, _ = file.WriteString("start\n")
	return slog.New(slog.NewTextHandler(file, nil)), nil
}

func dropAnnouncedLogger(path string) {
	logger, err := NewAnnouncedLogger(path)
	if err != nil {
		return
	}
	_ = logger
}
