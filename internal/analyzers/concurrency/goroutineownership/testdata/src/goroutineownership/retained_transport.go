package goroutineownership

import (
	"bufio"
	"io"
	"net"
	"strings"
)

// Gap: a worker writing to an io.Pipe whose reader the caller drains is not
// reported. Draining the peer may end the worker, so its completion is unknown.

// Closing a resource retained by the captured reader creates uncertain worker
// shutdown, not a join. A helper may retain its argument outside its result;
// this conservative boundary can miss an independently blocked worker.
func bufferedReaderClosedByCaller(connection net.Conn) {
	defer connection.Close()
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
}

func nestedCapturedConnectionCleanup(connection net.Conn) {
	go func() {
		defer connection.Close()
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(io.Discard, connection)
			close(done)
		}()
	}()
}

func bufferedReaderClosedAfterLaunch(connection net.Conn) {
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
	connection.Close()
}

func differentConnectionCleanup(connection, other net.Conn) {
	defer other.Close()
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
}

func ignoredTransport(io.Reader) *bufio.Reader {
	return bufio.NewReader(strings.NewReader("unrelated"))
}

func ignoredWrapperArgumentDoesNotSettle(connection net.Conn) {
	defer connection.Close()
	reader := ignoredTransport(connection)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
}

func transportClosedOnlyConditionally(connection net.Conn, stop bool) {
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
	if stop {
		connection.Close()
	}
}

func transportClosedBeforeLaunch(connection net.Conn) {
	reader := bufio.NewReader(connection)
	connection.Close()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
}

func transportReleaseDoesNotSettleErrorSend(connection net.Conn) {
	defer connection.Close()
	reader := bufio.NewReader(connection)
	done := make(chan error)
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		if _, err := reader.ReadString('\n'); err != nil {
			done <- err
		}
	}()
}
