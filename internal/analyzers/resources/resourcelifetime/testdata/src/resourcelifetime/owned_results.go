package resourcelifetime

import (
	"net"
	"net/textproto"
	"os"

	"resourcedep"
)

type protocolClient struct{ text *textproto.Conn }

func NewProtocolClient(conn net.Conn) *protocolClient {
	return &protocolClient{text: textproto.NewConn(conn)}
}

func (client *protocolClient) Close() error { return client.text.Close() }

// Wrapping an existing connection does not create a second socket obligation.
// The imported textproto constructor retains the same connection in its result.
func protocolClientUsesClosedConnection(conn net.Conn) {
	defer conn.Close()
	client := NewProtocolClient(conn)
	_ = client
}

// This file covers contracts synthesized from lifecycle summaries: a
// constructor summarized as returning a struct that owns a resource field, and
// a method of that type summarized as releasing it. The caller then owes that
// method. A wrapper around a caller's file and a type without a releasing
// method produce no contract.
// Nested custom-owner acquisition is deliberately not inferred from a Close
// method alone; a result may be a lazy or empty handle. Direct known-resource
// fields remain supported, including the imported Journal cases below.

type lazyHandle struct{ file *os.File }

func newLazyHandle() *lazyHandle { return &lazyHandle{} }

func (handle *lazyHandle) Close() error {
	if handle.file != nil {
		return handle.file.Close()
	}
	return nil
}

type lazyHolder struct{ handle *lazyHandle }

func newLazyHolder() *lazyHolder { return &lazyHolder{handle: newLazyHandle()} }

func (holder *lazyHolder) Close() error { return holder.handle.Close() }

func unopenedLazyHolderNeedsNoCleanup() {
	holder := newLazyHolder()
	_ = holder
}

func journalLeakedOnError(path string, lines []string) error {
	journal, err := resourcedep.OpenJournal(path) // want "owned resource from resourcedep.OpenJournal is not released on every return path"
	if err != nil {
		return err
	}
	for _, line := range lines {
		if err := journal.Append(line); err != nil {
			return err
		}
	}
	return journal.Close()
}

func journalClosedByDefer(path string, lines []string) error {
	journal, err := resourcedep.OpenJournal(path)
	if err != nil {
		return err
	}
	defer func() { _ = journal.Close() }()
	for _, line := range lines {
		if err := journal.Append(line); err != nil {
			return err
		}
	}
	return nil
}

func journalReturnedToCaller(path string) (*resourcedep.Journal, error) {
	return resourcedep.OpenJournal(path)
}

// A view stores the caller's file; the caller keeps the obligation and no
// contract attaches to the constructor.
func viewOverOwnedFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	view := resourcedep.NewView(file)
	_ = view
	return nil
}

// A type whose methods never release its field yields no cleanup the caller
// could be asked for.
func sinkWithoutRelease(path string) error {
	sink, err := resourcedep.OpenSink(path)
	if err != nil {
		return err
	}
	return sink.Flush()
}

// A view stored the caller's file and nothing on the view can release it, so
// returning the view does not transfer the file.
func viewReturnedWithoutClosing(path string) (*resourcedep.View, error) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return nil, err
	}
	return resourcedep.NewView(file), nil
}

// An adopted file is stored in a struct whose Close releases it, so the
// returned struct owns the file.
func fileAdoptedByOwner(path string) (*resourcedep.Journal, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return resourcedep.AdoptJournal(file), nil
}
