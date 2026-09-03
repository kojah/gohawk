// Package outer wraps a constructor from another package. The store that puts
// the argument into the returned value happens in that other package, so this
// package has no body to read and the answer comes from its summary. This is
// the shape encoding/csv takes through bufio and encoding/json takes through
// jsontext.
package outer

import (
	"io"

	"resourcelifetime/wrapping"
)

// Outer holds a wrapper that releases nothing, and releases nothing itself.
type Outer struct{ inner *wrapping.Reader }

func (o *Outer) Read(p []byte) (int, error) { return o.inner.Read(p) }

func NewOuter(source io.Reader) *Outer { return &Outer{inner: wrapping.NewReader(source)} }

// Sealed holds the same wrapper and offers a Close, but it was handed a plain
// reader, so that Close cannot be closing the caller's file. This is the shape
// compress/gzip takes: a Close that settles what the type constructed itself.
type Sealed struct{ inner *wrapping.Reader }

func (s *Sealed) Close() error { return nil }

func NewSealed(source io.Reader) *Sealed { return &Sealed{inner: wrapping.NewReader(source)} }

// Adopting delegates the store and carries a Close of its own, so neither the
// field the reader lands in nor the returned type decides the answer. What is
// left is whether the constructor discharged the obligation despite being
// handed a plain reader, which it did by asserting the reader to a closer.
// Without that question this reads as a view, and a caller that correctly
// handed its file over is reported.
type Adopting struct{ inner io.Reader }

func (a *Adopting) reset(source io.Reader) { a.inner = source }

func (a *Adopting) Close() error { return nil }

func NewAdopting(source io.Reader) *Adopting {
	if closer, ok := source.(io.Closer); ok {
		_ = closer.Close()
	}
	adopted := new(Adopting)
	adopted.reset(source)
	return adopted
}
