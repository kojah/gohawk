package resourcelifetime

import (
	"os"
	"os/exec"
)

// os.Pipe returns two files, and each end is its own obligation: closing the
// writer does not release the reader. The pipe's error result guards both,
// so the error branch holds neither end.

func pipeBothEndsClosed() error {
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	defer writer.Close()
	return nil
}

func pipeEndsReturned() (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return reader, writer, nil
}

// Handing an end to a command's descriptor list gives it to storage the
// analysis does not track, so that end is unknown; the other end is still
// this function's to close.
func pipeEndHandedToCommand(command *exec.Cmd) error {
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer writer.Close()
	command.ExtraFiles = append(command.ExtraFiles, reader)
	return command.Start()
}

func pipeReaderLeaked() error {
	reader, writer, err := os.Pipe() // want "owned resource from os.Pipe \\(read end\\) is not released on every return path"
	if err != nil {
		return err
	}
	_ = reader
	return writer.Close()
}

// The writer is closed after a successful start, but not when the start
// fails.
func pipeWriterLeakedOnStartError(command *exec.Cmd) error {
	reader, writer, err := os.Pipe() // want "owned resource from os.Pipe \\(write end\\) is not released on every return path"
	if err != nil {
		return err
	}
	defer reader.Close()
	command.Stdin = reader
	if err := command.Start(); err != nil {
		return err
	}
	return writer.Close()
}

// A project function named Pipe is not os.Pipe.
func Pipe() (*os.File, *os.File, error) { return os.Stdin, os.Stdout, nil }

func projectPipeIsNotAnAcquisition() {
	reader, writer, err := Pipe()
	_, _, _ = reader, writer, err
}
