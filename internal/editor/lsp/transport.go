package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Framing for JSON-RPC over a byte stream: each message is preceded by a
// Content-Length header block.
//
// Reading distinguishes a message that cannot be understood from a stream
// that cannot be followed. A body that is not a valid message has a known
// length, so the server can refuse it and read on. A header without a usable
// length leaves no way to find the next message, so the stream is abandoned.
//
// Writing never blocks the server's event loop for long. Messages are queued
// for one writer goroutine; a client that stops reading fills the queue, and
// after a bounded wait the connection is declared stalled so the server can
// stop instead of hanging where no exit request can reach it.

const (
	maxMessageBytes = 64 << 20
	maxHeaderBytes  = 8 << 10
	outgoingQueue   = 256
	stallTimeout    = 5 * time.Second
	flushTimeout    = time.Second
)

var (
	errStalled = errors.New("the client stopped reading")
	// errMalformedMessage marks a body that is not a valid message. The
	// stream is still in step, so reading can continue.
	errMalformedMessage = errors.New("malformed message")
)

type connection struct {
	// patience is how long a full queue is tolerated before the client is
	// declared to have stopped reading.
	patience time.Duration
	reader   *bufio.Reader
	outgoing chan []byte
	closed   chan struct{} // closed when no more messages will be sent
	written  chan struct{} // closed when the writer goroutine has finished
	stalled  chan struct{} // closed when the client stopped reading
	stall    sync.Once
	closing  sync.Once
}

func newConnection(input io.Reader, output io.Writer) *connection {
	c := &connection{
		patience: stallTimeout,
		reader:   bufio.NewReaderSize(input, maxHeaderBytes),
		outgoing: make(chan []byte, outgoingQueue),
		closed:   make(chan struct{}),
		written:  make(chan struct{}),
		stalled:  make(chan struct{}),
	}
	go c.writeLoop(output)
	return c
}

func (c *connection) writeLoop(output io.Writer) {
	defer close(c.written)
	send := func(frame []byte) {
		if _, err := output.Write(frame); err != nil {
			c.markStalled()
		}
	}
	for {
		select {
		case frame := <-c.outgoing:
			send(frame)
		case <-c.closed:
			// Let what was already queued leave, then stop.
			for {
				select {
				case frame := <-c.outgoing:
					send(frame)
				default:
					return
				}
			}
		}
	}
}

func (c *connection) markStalled() {
	c.stall.Do(func() { close(c.stalled) })
}

// close stops accepting messages and gives queued ones a moment to leave.
func (c *connection) close() {
	c.closing.Do(func() { close(c.closed) })
	select {
	case <-c.written:
	case <-time.After(flushTimeout):
	}
}

func (c *connection) read() (*message, error) {
	length, err := c.readHeader()
	if err != nil {
		return nil, err
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, err
	}
	var decoded message
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("%w: %w", errMalformedMessage, err)
	}
	return &decoded, nil
}

// readHeader returns the body length the header block announces.
func (c *connection) readHeader() (int, error) {
	length := -1
	for {
		line, err := c.reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			return 0, fmt.Errorf("header line longer than %d bytes", maxHeaderBytes)
		}
		if err != nil {
			return 0, err
		}
		text := strings.TrimRight(string(line), "\r\n")
		if text == "" {
			break
		}
		name, value, found := strings.Cut(text, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		length, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("invalid Content-Length %q", value)
		}
	}
	if length < 0 || length > maxMessageBytes {
		return 0, fmt.Errorf("missing or oversized Content-Length: %d", length)
	}
	return length, nil
}

func (c *connection) write(outgoing *message) error {
	outgoing.JSONRPC = "2.0"
	body, err := json.Marshal(outgoing)
	if err != nil {
		return err
	}
	frame := append(fmt.Appendf(nil, "Content-Length: %d\r\n\r\n", len(body)), body...)
	select {
	case <-c.stalled:
		return errStalled
	case <-c.closed:
		return errStalled
	case c.outgoing <- frame:
		return nil
	default:
	}
	// The queue is full: the client is slow or has stopped reading. Wait a
	// bounded time, then declare the connection stalled.
	patience := time.NewTimer(c.patience)
	defer patience.Stop()
	select {
	case c.outgoing <- frame:
		return nil
	case <-c.stalled:
		return errStalled
	case <-c.closed:
		return errStalled
	case <-patience.C:
		c.markStalled()
		return errStalled
	}
}

func (c *connection) notify(method string, params any) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(&message{Method: method, Params: encoded})
}

func (c *connection) reply(id *json.RawMessage, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.write(&message{ID: id, Result: encoded})
}

func (c *connection) replyError(id *json.RawMessage, code int, text string) error {
	if id == nil {
		null := json.RawMessage("null")
		id = &null
	}
	return c.write(&message{ID: id, Error: &responseError{Code: code, Message: text}})
}

// request sends a server-to-client request. The server does not wait for the
// reply: the one request it makes only asks the client to show something.
func (c *connection) request(id int, method string, params any) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	raw := json.RawMessage(strconv.Itoa(id))
	return c.write(&message{ID: &raw, Method: method, Params: encoded})
}
