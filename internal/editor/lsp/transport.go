package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Framing for JSON-RPC over a byte stream: each message is preceded by a
// Content-Length header block. Writes are serialized because run completions
// and request replies are produced by different goroutines.

const maxMessageBytes = 64 << 20

type connection struct {
	reader *bufio.Reader
	mutex  sync.Mutex
	writer io.Writer
}

func newConnection(input io.Reader, output io.Writer) *connection {
	return &connection{reader: bufio.NewReader(input), writer: output}
}

func (c *connection) read() (*message, error) {
	length := -1
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		length, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid Content-Length %q", value)
		}
	}
	if length < 0 || length > maxMessageBytes {
		return nil, fmt.Errorf("missing or oversized Content-Length: %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, err
	}
	var decoded message
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	return &decoded, nil
}

func (c *connection) write(outgoing *message) error {
	outgoing.JSONRPC = "2.0"
	body, err := json.Marshal(outgoing)
	if err != nil {
		return err
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if _, err := fmt.Fprintf(c.writer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.writer.Write(body)
	return err
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
	return c.write(&message{ID: id, Error: &responseError{Code: code, Message: text}})
}

// request sends a server-to-client request. The server does not wait for the
// reply: the two requests it makes only ask the client to show something.
func (c *connection) request(id int, method string, params any) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	raw := json.RawMessage(strconv.Itoa(id))
	return c.write(&message{ID: &raw, Method: method, Params: encoded})
}
