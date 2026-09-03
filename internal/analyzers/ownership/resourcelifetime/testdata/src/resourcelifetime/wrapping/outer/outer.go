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

// Sealed holds the same wrapper but offers a Close, so it may release what it
// holds and the proof stops rather than guess which field that is.
type Sealed struct{ inner *wrapping.Reader }

func (s *Sealed) Close() error { return nil }

func NewSealed(source io.Reader) *Sealed { return &Sealed{inner: wrapping.NewReader(source)} }
