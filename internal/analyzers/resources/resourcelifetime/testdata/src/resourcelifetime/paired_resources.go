package resourcelifetime

import (
	"os"
	"resourcedep"
)

// A multi-result acquisition can return two owned resources and a final error.
// On the error path neither file exists; on success both are transferred to
// the returned aggregate. Lilipod's termios.Pty wrapper has this shape:
// https://github.com/89luca89/lilipod/blob/872755a7cef33c238ea2d11b2310b3116944eb48/ptyagent/pty.go#L134-L146
type pipeOwner struct {
	reader *os.File
	writer *os.File
}

func returnedPipeOwner() (*pipeOwner, error) {
	reader, writer, err := resourcedep.OpenPair("state")
	if err != nil {
		return nil, err
	}
	return &pipeOwner{reader: reader, writer: writer}, nil
}

// The same three-result shape remains a leak when neither file is returned
// or closed on the success path.
func discardedPipePair() error {
	reader, writer, err := resourcedep.OpenPair("state") // want "owned resource from resourcedep.OpenPair is not released on every return path"
	if err != nil {
		return err
	}
	_, _ = reader, writer
	return nil
}

// A later comparison against an unrelated error cannot erase ownership of
// resources already acquired by the multi-result call.
func unrelatedErrorDoesNotSettlePipe(other error) error {
	reader, writer, err := resourcedep.OpenPair("state") // want "owned resource from resourcedep.OpenPair is not released on every return path"
	if err != nil {
		return err
	}
	if other != nil {
		return other
	}
	_, _ = reader, writer
	return nil
}
