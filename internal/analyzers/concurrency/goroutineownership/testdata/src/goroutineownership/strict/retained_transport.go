package goroutineownershipstrict

import (
	"bufio"
	"context"
	"io"
	"net"
)

type contextEnvelope struct{ ctx context.Context }
type envelopeHandler interface{ Handle(*contextEnvelope) }

func RetainContext(ctx context.Context) *contextEnvelope { return &contextEnvelope{ctx: ctx} }

func retainedContextStillNeedsJoin(ctx context.Context, handler envelopeHandler) {
	request := RetainContext(ctx)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		handler.Handle(request)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Retained transport shutdown is uncertainty about lifecycle, not a join.
func bufferedReaderStillNeedsJoin(connection net.Conn) {
	defer connection.Close()
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
}

func pipePeerStillNeedsJoin() {
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		writer.Write([]byte("data"))
	}()
	_, _ = io.ReadAll(reader)
}
