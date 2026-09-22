package goroutineownership

import "io"

// Passing a pipe peer into a helper makes its worker's shutdown uncertain;
// it does not prove a join. Merely creating/storing that peer is not a handoff.
func pipePeerConsumedAfterLaunch() {
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = writer.Write([]byte("data"))
	}()
	_, _ = io.ReadAll(reader)
}

func pipePeerClosedBeforeLaunch() {
	reader, writer := io.Pipe()
	reader.Close()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = writer.Write([]byte("data"))
	}()
}

func ignoredPipePeer(io.Reader) {}

func ignoredPeerDoesNotSettle() {
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = writer.Write([]byte("data"))
	}()
	ignoredPipePeer(reader)
}

func unrelatedPipeDoesNotSettle() {
	_, writer := io.Pipe()
	other, _ := io.Pipe()
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = writer.Write([]byte("data"))
	}()
	_, _ = io.ReadAll(other)
}

func pipePeerOnlyStoredLocally() {
	reader, writer := io.Pipe()
	request := struct{ body io.Reader }{reader}
	done := make(chan struct{})
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, _ = writer.Write([]byte("data"))
	}()
	_ = request
}

func pipePeerDoesNotSettleErrorSend() {
	reader, writer := io.Pipe()
	done := make(chan error)
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, err := writer.Write([]byte("data"))
		done <- err
	}()
	_, _ = io.ReadAll(reader)
}

func pipePeerDoesNotSettleErrorSelect(other chan error) {
	reader, writer := io.Pipe()
	done := make(chan error)
	go func() { // want "goroutine is not joined on every return path"
		defer close(done)
		_, err := writer.Write([]byte("data"))
		select {
		case done <- err:
		case other <- err:
		}
	}()
	_, _ = io.ReadAll(reader)
}
