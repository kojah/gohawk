package goroutineownership

import (
	"bufio"
	"net"
)

func connectionAndCleanup() (net.Conn, func()) {
	connection, _ := net.Dial("tcp", "unused")
	return connection, func() { connection.Close() }
}

func connectionAndUnrelatedCleanup() (net.Conn, func()) {
	connection, _ := net.Dial("tcp", "unused")
	other, _ := net.Dial("tcp", "unrelated")
	return connection, func() { other.Close() }
}

func returnedCleanupBoundsReader() {
	connection, cleanup := connectionAndCleanup()
	defer cleanup()
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
}

func unrelatedReturnedCleanupDoesNotSettle() {
	connection, cleanup := connectionAndUnrelatedCleanup()
	defer cleanup()
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
}

func returnedCleanupMustBeInvoked() {
	connection, cleanup := connectionAndCleanup()
	reader := bufio.NewReader(connection)
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = reader.ReadString('\n')
	}()
	_ = cleanup
}
